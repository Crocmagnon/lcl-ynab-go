package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/carlmjohnson/requests"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	milliUnit     = 1000
	apiTimeout    = 10 * time.Second
	lclDateFormat = "02/01/06"
	lclDateLen    = len(lclDateFormat)
)

var errRequiredFlag = errors.New("flag is required")

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:], os.Stdout, http.DefaultClient); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, httpClient *http.Client) error {
	var (
		filename  string
		budgetID  string
		accountID string
		token     string
		webhook   string
		verbose   bool
	)

	err := parseFlags(args, &filename, &budgetID, &accountID, &token, &webhook, &verbose)
	if err != nil {
		return err
	}

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}

	transactions, reconciled, err := convert(file, accountID)
	if err != nil {
		return fmt.Errorf("converting to YNAB transactions: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(stdout, "transactions:\n%+v\n\n", transactions)
	}

	_, _ = fmt.Fprintf(stdout, "reconciled: %v€\n", reconciledString(reconciled))

	duplicateCount, err := push(ctx, httpClient, transactions, budgetID, token)
	if err != nil {
		return fmt.Errorf("pushing to YNAB: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "successfully pushed %d transaction(s)\n", len(transactions))
	_, _ = fmt.Fprintf(stdout, "found %d duplicate(s)\n", duplicateCount)

	if webhook != "" {
		if err := send(ctx, webhook, reconciled); err != nil {
			return fmt.Errorf("sending webhook: %w", err)
		}
	}

	return nil
}

func parseFlags(args []string, filename, budgetID, accountID, token, webhook *string, verbose *bool) error {
	flagset := flag.NewFlagSet("", flag.ExitOnError)
	flagset.StringVar(filename, "f", "", "CSV file to parse")
	flagset.StringVar(budgetID, "b", "", "Budget ID")
	flagset.StringVar(accountID, "a", "", "Account ID")
	flagset.StringVar(token, "t", "", "Token")
	flagset.StringVar(webhook, "w", "", "Home Assistant webhook URL")
	flagset.BoolVar(verbose, "v", false, "Verbose output")

	err := flagset.Parse(args)
	if err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	switch {
	case *filename == "":
		return fmt.Errorf("%w: -f", errRequiredFlag)
	case *budgetID == "":
		return fmt.Errorf("%w: -b", errRequiredFlag)
	case *accountID == "":
		return fmt.Errorf("%w: -a", errRequiredFlag)
	case *token == "":
		return fmt.Errorf("%w: -t", errRequiredFlag)
	}

	return nil
}

func convert(reader io.Reader, accountID string) ([]Transaction, int, error) {
	if reader == nil {
		return nil, 0, nil
	}

	transformer := unicode.BOMOverride(encoding.Nop.NewDecoder())

	csvReader := csv.NewReader(transform.NewReader(reader, transformer))
	csvReader.Comma = ';'

	var transactions []Transaction

	importIDs := make(map[string]int)

	for {
		record, err := csvReader.Read()

		if errors.Is(err, io.EOF) {
			break
		}

		if errors.Is(err, csv.ErrFieldCount) {
			return transactions, getReconciled(record), nil
		}

		if err != nil {
			return nil, 0, fmt.Errorf("reading csv line: %w", err)
		}

		transaction, err := convertLine(record, accountID, importIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("converting line: %w", err)
		}

		transactions = append(transactions, *transaction)
	}

	return transactions, 0, nil
}

func convertLine(record []string, accountID string, importIDs map[string]int) (*Transaction, error) {
	date, err := time.Parse("02/01/2006", record[0])
	if err != nil {
		return nil, fmt.Errorf("parsing date: %w", err)
	}

	amount, err := getAmount(record[1])
	if err != nil {
		return nil, err
	}

	recordString := record[4]
	if amount > 0 {
		recordString = record[5]
	}

	if specificDate, ok := getDate(recordString); ok {
		date = specificDate
	}

	formattedDate := date.Format("2006-01-02")

	payee := getPayee(recordString)

	transaction := &Transaction{
		AccountID: accountID,
		Date:      formattedDate,
		PayeeName: payee,
		Memo:      recordString,
		Amount:    amount,
		ImportID:  createImportID(amount, formattedDate, importIDs),
		Cleared:   "cleared",
	}

	return transaction, nil
}

func getDate(recordString string) (time.Time, bool) {
	if len(recordString) < lclDateLen {
		return time.Time{}, false
	}

	date, err := time.Parse(lclDateFormat, recordString[len(recordString)-8:])
	if err != nil {
		return time.Time{}, false
	}

	return date, true
}

func getPayee(recordString string) string {
	if len(recordString) < lclDateLen {
		return recordString
	}

	_, err := time.Parse(lclDateFormat, recordString[len(recordString)-lclDateLen:])
	if err != nil {
		return recordString
	}

	return strings.TrimSpace(recordString[:len(recordString)-lclDateLen])
}

func getAmount(amnt string) (int, error) {
	amntFloat, err := strconv.ParseFloat(strings.ReplaceAll(amnt, ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("parsing amount: %w", err)
	}

	return int(amntFloat * milliUnit), nil
}

func getReconciled(record []string) int {
	amount, err := getAmount(record[1])
	if err != nil {
		return 0
	}

	return amount
}

func createImportID(amount int, date string, importIDs map[string]int) string {
	importID := fmt.Sprintf("YNAB:%v:%v", amount, date)
	occurrence := importIDs[importID] + 1
	importIDs[importID] = occurrence

	return fmt.Sprintf("%v:%v", importID, occurrence)
}

func push(
	ctx context.Context,
	client *http.Client,
	transactions []Transaction,
	budgetID, token string,
) (duplicateCount int, err error) {
	if len(transactions) == 0 {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	var (
		resp    TransactionsResponse
		errResp bytes.Buffer
	)

	//nolint:bodyclose // reported https://github.com/earthboundkid/requests/discussions/121
	err = requests.URL("https://api.youneedabudget.com/").
		Client(client).
		Pathf("/v1/budgets/%s/transactions", budgetID).
		Header("Authorization", fmt.Sprintf("Bearer %v", token)).
		Method(http.MethodPost).
		AddValidator(requests.ValidatorHandler(requests.DefaultValidator, requests.ToBytesBuffer(&errResp))).
		BodyJSON(TransactionsPayload{Transactions: transactions}).
		ToJSON(&resp).
		Fetch(ctx)
	if err != nil {
		return 0, fmt.Errorf("pushing transactions: %w - %v", err, errResp.String())
	}

	return len(resp.Data.DuplicateImportIDs), nil
}

func send(ctx context.Context, webhook string, reconciled int) error {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	type Payload struct {
		Reconciled string `json:"reconciled"`
	}

	err := requests.URL(webhook).
		Method(http.MethodPost).
		BodyJSON(Payload{Reconciled: reconciledString(reconciled)}).
		Fetch(ctx)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}

	return nil
}

func reconciledString(amnt int) string {
	return fmt.Sprintf("%.2f", float64(amnt)/milliUnit)
}
