package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
)

type request struct {
	MerchantID string `json:"merchant_id"`
	Credential *struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"browser_credential"`
}

func main() {
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil || input.MerchantID == "" {
		fail("checker input requires merchant_id")
	}
	cdpURL := env("NEKO_CDP_URL", "http://neko:9222")
	portalURL := env("WEBWRIGHT_PORTAL_URL", "https://merchant.qris.interactive.co.id")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	allocator, cancelAllocator := chromedp.NewRemoteAllocator(ctx, cdpURL)
	defer cancelAllocator()
	// Neko owns the visual Chromium tab. Attach to that tab rather than asking
	// Chromium to create a background target, which it rejects for remote CDP.
	targets, err := target.GetTargets().Do(cdp.WithExecutor(allocator, nil))
	if err != nil {
		fail("cannot inspect Neko browser: " + err.Error())
	}
	var targetID target.ID
	for _, item := range targets {
		if item.Type == "page" {
			targetID = item.TargetID
			break
		}
	}
	if targetID == "" {
		fail("Neko has no browser page to attach")
	}
	browserCtx, cancelBrowser := chromedp.NewContext(allocator, chromedp.WithTargetID(targetID))
	defer cancelBrowser()

	// This targets the same persistent Chromium profile displayed by Neko.
	if err := chromedp.Run(browserCtx, chromedp.Navigate(portalURL)); err != nil {
		fail("cannot open the Neko browser: " + err.Error())
	}
	if input.Credential != nil && input.Credential.Email != "" && input.Credential.Password != "" {
		_ = chromedp.Run(browserCtx,
			chromedp.SendKeys(`input[type="email"], input[name*="email" i], input[type="text"]`, input.Credential.Email, chromedp.ByQuery),
			chromedp.SendKeys(`input[type="password"]`, input.Credential.Password, chromedp.ByQuery),
		)
	}
	if os.Getenv("WEBWRIGHT_MANUAL_LOGIN") == "true" {
		// CAPTCHA is completed interactively in Neko. Do not click or solve it here.
		if err := chromedp.Run(browserCtx, chromedp.WaitNotPresent(`input[type="password"]`, chromedp.ByQuery)); err != nil {
			fail("manual login did not complete in Neko: " + err.Error())
		}
	}
	fmt.Print(`{"transactions":[]}` + "\n")
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
