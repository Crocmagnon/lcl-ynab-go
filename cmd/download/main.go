package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

const (
	wantIdentifierLen = 10
	wantPasswordLen   = 6
)

var errInvalidLen = errors.New("invalid length")

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	var (
		identifier    string
		password      string
		outputFile    string
		screenshotDir string
		headless      bool
	)

	err := parseFlags(args, &identifier, &password, &outputFile, &screenshotDir, &headless)
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

	playw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("launching playwright: %w", err)
	}

	defer playw.Stop() //nolint:errcheck

	browser, err := playw.Firefox.Launch(playwright.BrowserTypeLaunchOptions{
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
		saveScreenshot(page, stderr, screenshotDir)
		return err
	}

	return nil
}

func saveScreenshot(page playwright.Page, stderr io.Writer, dir string) {
	img, err := page.Screenshot()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error saving screenshot:", err)
		return
	}

	const perm = 0o755
	_ = os.MkdirAll(dir, perm)

	file, err := os.Create(filepath.Join(dir, "screenshot.png"))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error creating screenshot file:", err)
		return
	}

	defer file.Close()
	_, _ = file.Write(img)
}

func parseFlags(args []string, identifier, password, outputFile, screenshotDir *string, headless *bool) error {
	flagset := flag.NewFlagSet("", flag.ExitOnError)
	flagset.StringVar(identifier, "i", "", "Bank identifier")
	flagset.StringVar(password, "p", "", "Bank password")
	flagset.StringVar(outputFile, "o", "", "Output file")
	flagset.StringVar(screenshotDir, "screenshots", "screenshots", "Output file")
	flagset.BoolVar(headless, "headless", false, "Headless mode")

	err := flagset.Parse(args)
	if err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if len(*identifier) != wantIdentifierLen {
		return fmt.Errorf("%w for identifier: %d, want %d", errInvalidLen, len(*identifier), wantIdentifierLen)
	}

	if len(*password) != wantPasswordLen {
		return fmt.Errorf("%w for password: %d, want %d", errInvalidLen, len(*password), wantPasswordLen)
	}

	return nil
}

func downloadFile(page playwright.Page, identifier, password, outputFile string) error {
	if err := login(page, identifier, password); err != nil {
		return fmt.Errorf("logging in: %w", err)
	}

	if err := navigateToForm(page); err != nil {
		return fmt.Errorf("navigating to form: %w", err)
	}

	if err := fillForm(page); err != nil {
		return fmt.Errorf("filling form: %w", err)
	}

	if err := downloadAndSave(page, outputFile); err != nil {
		return fmt.Errorf("downloading and saving: %w", err)
	}

	return nil
}

func login(page playwright.Page, identifier, password string) error {
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

	return nil
}

func navigateToForm(page playwright.Page) error {
	if err := page.Locator(".extended-zone").First().Click(); err != nil {
		return fmt.Errorf("clicking account: %w", err)
	}

	if err := page.Locator("#export-button").First().Click(); err != nil {
		return fmt.Errorf("clicking export button: %w", err)
	}

	return nil
}

func fillForm(page playwright.Page) error {
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

	return nil
}

func downloadAndSave(page playwright.Page, outputFile string) error {
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
