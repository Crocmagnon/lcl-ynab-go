package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"github.com/Crocmagnon/ynab-go/internal/ynab"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	var (
		filename  string
		budgetID  string
		accountID string
		token     string
	)

	err := parseFlags(args, &filename, &budgetID, &accountID, &token)
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

	fmt.Printf("transactions:\n%+v\n", transactions)
	fmt.Printf("reconciled:\n%v\n", reconciled)

	if err := push(transactions); err != nil {
		return fmt.Errorf("pushing to YNAB: %w", err)
	}

	return nil
}

func parseFlags(args []string, filename, budgetID, accountID, token *string) error {
	flagset := flag.NewFlagSet("", flag.ExitOnError)
	flagset.StringVar(filename, "f", "", "CSV file to parse")
	flagset.StringVar(budgetID, "b", "", "Budget ID")
	flagset.StringVar(accountID, "a", "", "Account ID")
	flagset.StringVar(token, "t", "", "Token")

	err := flagset.Parse(args)
	if err != nil {
		return fmt.Errorf("parsing flags: %w", err)
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

	formattedDate := date.Format("2006-01-02")

	transaction := &ynab.Transaction{
		AccountId: accountID,
		Date:      formattedDate,
		PayeeName: record[5],
		Memo:      record[5],
		Amount:    amount,
		ImportId:  createImportID(amount, formattedDate, importIDs),
		Cleared:   "cleared",
	}

	return transaction, nil
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

func push(transactions []ynab.Transaction) error {
	return fmt.Errorf("not implemented")
}
