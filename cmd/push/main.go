package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"github.com/Crocmagnon/ynab-go/internal/ynab"
	"github.com/carlmjohnson/requests"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var errRequiredFlag = errors.New("flag is required")

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	var (
		filename  string
		budgetID  string
		accountID string
		token     string
		verbose   bool
	)

	err := parseFlags(args, &filename, &budgetID, &accountID, &token, &verbose)
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
		fmt.Printf("transactions:\n%+v\n\n", transactions)
	}
	fmt.Printf("reconciled:%.2f€\n", float64(reconciled)/100.0)

	duplicateCount, err := push(ctx, transactions, budgetID, token)
	if err != nil {
		return fmt.Errorf("pushing to YNAB: %w", err)
	}

	fmt.Fprintf(stdout, "successfully pushed %d transaction(s)\n", len(transactions))
	fmt.Fprintf(stdout, "found %d duplicate(s)\n", duplicateCount)

	return nil
}

func parseFlags(args []string, filename, budgetID, accountID, token *string, verbose *bool) error {
	flagset := flag.NewFlagSet("", flag.ExitOnError)
	flagset.StringVar(filename, "f", "", "CSV file to parse")
	flagset.StringVar(budgetID, "b", "", "Budget ID")
	flagset.StringVar(accountID, "a", "", "Account ID")
	flagset.StringVar(token, "t", "", "Token")
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

func convert(reader io.Reader, accountID string) ([]ynab.Transaction, int, error) {
	transformer := unicode.BOMOverride(encoding.Nop.NewDecoder())

	csvReader := csv.NewReader(transform.NewReader(reader, transformer))
	csvReader.Comma = ';'

	var transactions []ynab.Transaction

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

func convertLine(record []string, accountID string, importIDs map[string]int) (*ynab.Transaction, error) {
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

	transaction := &ynab.Transaction{
		AccountId: accountID,
		Date:      formattedDate,
		PayeeName: payee,
		Memo:      recordString,
		Amount:    amount,
		ImportId:  createImportID(amount, formattedDate, importIDs),
		Cleared:   "cleared",
	}

	return transaction, nil
}

func getDate(recordString string) (time.Time, bool) {
	if len(recordString) < 8 {
		return time.Time{}, false
	}

	date, err := time.Parse("02/01/06", recordString[len(recordString)-8:])
	if err != nil {
		return time.Time{}, false
	}

	return date, true
}

func getPayee(recordString string) string {
	if len(recordString) < 8 {
		return recordString
	}

	_, err := time.Parse("02/01/06", recordString[len(recordString)-8:])
	if err != nil {
		return recordString
	}

	return strings.TrimSpace(recordString[:len(recordString)-8])
}

func getAmount(amnt string) (int, error) {
	amntFloat, err := strconv.ParseFloat(strings.ReplaceAll(amnt, ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("parsing amount: %w", err)
	}

	amount := int(amntFloat * 1000)
	return amount, nil
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

func push(ctx context.Context, transactions []ynab.Transaction, budgetID, token string) (int, error) {
	var (
		resp    ynab.TransactionsResponse
		errResp bytes.Buffer
	)

	err := requests.URL("https://api.youneedabudget.com/").
		Pathf("/v1/budgets/%s/transactions", budgetID).
		Header("Authorization", fmt.Sprintf("Bearer %v", token)).
		Method(http.MethodPost).
		AddValidator(requests.ValidatorHandler(requests.DefaultValidator, requests.ToBytesBuffer(&errResp))).
		BodyJSON(ynab.TransactionsPayload{Transactions: transactions}).
		ToJSON(&resp).
		Fetch(ctx)
	if err != nil {
		return 0, fmt.Errorf("pushing transactions: %w - %v", err, errResp.String())
	}

	return len(resp.Data.DuplicateImportIds), nil
}
