package main

import (
	"flag"
	"fmt"
	"github.com/playwright-community/playwright-go"
	"io"
	"os"
	"time"
)

const (
	wantIdentifierLen = 10
	wantPasswordLen   = 6
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	var (
		identifier string
		password   string
		outputFile string
		headless   bool
	)

	err := parseFlags(args, &identifier, &password, &outputFile, &headless)
	if err != nil {
		return err
	}

	err = playwright.Install(&playwright.RunOptions{
		Browsers: []string{"firefox"},
		Stdout:   stdout,
		Stderr:   stderr,
	})
	if err != nil {
		return fmt.Errorf("installing playwright: %w", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("launching playwright: %w", err)
	}

	defer pw.Stop()

	browser, err := pw.Firefox.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})
	if err != nil {
		return fmt.Errorf("launching Firefox: %w", err)
	}

	defer browser.Close()

	context, err := browser.NewContext()
	if err != nil {
		return fmt.Errorf("creating context: %w", err)
	}

	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		return fmt.Errorf("creating page: %w", err)
	}

	defer page.Close()

	if err := downloadFile(page, identifier, password, outputFile); err != nil {
		return err
	}

	return nil
}

func parseFlags(args []string, identifier, password, outputFile *string, headless *bool) error {
	flagset := flag.NewFlagSet("", flag.ExitOnError)
	flagset.StringVar(identifier, "i", "", "Bank identifier")
	flagset.StringVar(password, "p", "", "Bank password")
	flagset.StringVar(outputFile, "o", "", "Output file")
	flagset.BoolVar(headless, "headless", false, "Headless mode")

	err := flagset.Parse(args)
	if err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if len(*identifier) != wantIdentifierLen {
		return fmt.Errorf("invalid identifier length %d, want %d", len(*identifier), wantIdentifierLen)
	}

	if len(*password) != wantPasswordLen {
		return fmt.Errorf("invalid password length %d, want %d", len(*password), wantPasswordLen)
	}

	return nil
}

func downloadFile(page playwright.Page, identifier, password, outputFile string) error {
	_, err := page.Goto("https://monespace.lcl.fr/connexion")
	if err != nil {
		return fmt.Errorf("going to: %w", err)
	}

	_ = page.Locator("#popin_tc_privacy_button_2").Click() // we don't care about this error

	if err := page.Locator("#identifier").Fill(identifier); err != nil {
		return fmt.Errorf("typing identifier: %w", err)
	}

	if err := page.Locator(".app-cta-button").First().Click(); err != nil {
		return fmt.Errorf("clicking login button: %w", err)
	}

	for _, char := range password {
		if err := page.Locator(fmt.Sprintf(".pad-button[value='%s']", string(char))).Click(); err != nil {
			return fmt.Errorf("clicking pad button: %w", err)
		}
	}

	if err := page.Locator(".app-cta-button").First().Click(); err != nil {
		return fmt.Errorf("clicking login button: %w", err)
	}

	if err := page.Locator(".extended-zone").First().Click(); err != nil {
		return fmt.Errorf("clicking account: %w", err)
	}

	if err := page.Locator("#export-button").First().Click(); err != nil {
		return fmt.Errorf("clicking export button: %w", err)
	}

	end := time.Now().UTC().AddDate(0, 0, -1)
	start := end.AddDate(0, -1, 0)

	if err := page.Locator("#mat-input-0").Fill(start.Format("02/01/2006")); err != nil {
		return fmt.Errorf("filling start date: %w", err)
	}

	if err := page.Locator("#mat-input-1").Fill(end.Format("02/01/2006")); err != nil {
		return fmt.Errorf("filling start date: %w", err)
	}

	if err := page.Locator("ui-desktop-select button").Click(); err != nil {
		return fmt.Errorf("clicking file type selector button: %w", err)
	}

	// 0 : CSV
	// 2 : OFX
	if err := page.Locator("ui-select-list ul li").Nth(0).Click(); err != nil {
		return fmt.Errorf("clicking file format button: %w", err)
	}

	download, err := page.ExpectDownload(func() error {
		return page.Locator(".download-button").Click()
	})
	if err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}

	if err := download.SaveAs(outputFile); err != nil {
		return fmt.Errorf("saving download file: %w", err)
	}
	return nil
}
