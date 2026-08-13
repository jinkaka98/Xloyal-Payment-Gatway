package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type request struct {
	MerchantID string `json:"merchant_id"`
	Credential *struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"browser_credential"`
}

type pageTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func main() {
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil || input.MerchantID == "" {
		fail("checker input requires merchant_id")
	}
	cdpURL := env("NEKO_CDP_URL", "http://neko:9222")
	portalURL := env("WEBWRIGHT_PORTAL_URL", "https://merchant.qris.interactive.co.id")
	page := portalPage(cdpURL, portalURL)
	activatePage(cdpURL, page.ID)

	if input.Credential != nil && input.Credential.Email != "" && input.Credential.Password != "" && isLoginPage(page.URL) {
		fillLogin(page.WebSocketDebuggerURL, input.Credential.Email, input.Credential.Password)
	}
	if os.Getenv("WEBWRIGHT_MANUAL_LOGIN") == "true" {
		waitForPortalLogin(cdpURL, page.ID)
		fmt.Print(`{"transactions":[]}` + "\n")
		return
	}
	page = pageByID(cdpURL, page.ID)
	if isLoginPage(page.URL) {
		fail("portal login or reCAPTCHA is required in Neko")
	}
	fmt.Print(`{"transactions":[]}` + "\n")
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func pages(cdpURL string) []pageTarget {
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(strings.TrimRight(cdpURL, "/") + "/json/list")
	if err != nil {
		fail("cannot inspect Neko browser: " + err.Error())
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fail("cannot inspect Neko browser: " + response.Status)
	}
	var result []pageTarget
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		fail("cannot decode Neko browser tabs: " + err.Error())
	}
	return result
}

func portalPage(cdpURL, portalURL string) pageTarget {
	for _, page := range pages(cdpURL) {
		if page.Type == "page" && strings.Contains(page.URL, "merchant.qris.interactive.co.id") {
			return page
		}
	}
	request, err := http.NewRequest(http.MethodPut, strings.TrimRight(cdpURL, "/")+"/json/new?"+url.QueryEscape(portalURL), nil)
	if err != nil {
		fail("cannot prepare portal browser: " + err.Error())
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		fail("cannot open portal in Neko: " + err.Error())
	}
	defer response.Body.Close()
	var page pageTarget
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&page) != nil {
		fail("cannot open portal in Neko")
	}
	return page
}

func pageByID(cdpURL, id string) pageTarget {
	for _, page := range pages(cdpURL) {
		if page.ID == id {
			return page
		}
	}
	fail("Neko portal tab was closed")
	return pageTarget{}
}

func activatePage(cdpURL, id string) {
	response, err := http.Get(strings.TrimRight(cdpURL, "/") + "/json/activate/" + id)
	if err != nil || response.StatusCode != http.StatusOK {
		fail("cannot activate the Neko portal tab")
	}
	response.Body.Close()
}

func fillLogin(wsURL, email, password string) {
	conn, _, _, err := ws.Dial(context.Background(), wsURL)
	if err != nil {
		fail("cannot control the Neko portal tab: " + err.Error())
	}
	defer conn.Close()
	expression := fmt.Sprintf(`(() => {
		const set = (el, value) => { if (!el) return false; const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set; setter.call(el, value); el.dispatchEvent(new Event('input', {bubbles:true})); el.dispatchEvent(new Event('change', {bubbles:true})); return true; };
		const email = document.querySelector('input[type=email], input[name*=email i], input[type=text]');
		const password = document.querySelector('input[type=password]');
		set(email, %s); set(password, %s);
		const submit = document.querySelector('button[type=submit], input[type=submit]'); if (submit) submit.click();
		return Boolean(email && password);
	})()`, jsString(email), jsString(password))
	command, _ := json.Marshal(map[string]any{"id": 1, "method": "Runtime.evaluate", "params": map[string]any{"expression": expression, "awaitPromise": true}})
	if err := wsutil.WriteClientText(conn, command); err != nil {
		fail("cannot fill portal credentials: " + err.Error())
	}
}

func waitForPortalLogin(cdpURL, id string) {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		if !isLoginPage(pageByID(cdpURL, id).URL) {
			return
		}
		time.Sleep(2 * time.Second)
	}
	fail("manual login did not complete in Neko before timeout")
}

func isLoginPage(value string) bool {
	lower := strings.ToLower(value)
	return lower == "" || strings.Contains(lower, "/login")
}

func jsString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
