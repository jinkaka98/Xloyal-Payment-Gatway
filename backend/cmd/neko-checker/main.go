package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
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
	targetID := activePageTarget(cdpURL)
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

func activePageTarget(cdpURL string) target.ID {
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(strings.TrimRight(cdpURL, "/") + "/json/list")
	if err != nil {
		fail("cannot inspect Neko browser: " + err.Error())
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fail("cannot inspect Neko browser: " + response.Status)
	}
	var pages []struct {
		ID   target.ID `json:"id"`
		Type string    `json:"type"`
	}
	if err := json.NewDecoder(response.Body).Decode(&pages); err != nil {
		fail("cannot decode Neko browser tabs: " + err.Error())
	}
	for _, page := range pages {
		if page.Type == "page" {
			return page.ID
		}
	}
	fail("Neko has no browser page to attach")
	return ""
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
