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
	if input.Credential != nil && input.Credential.Email != "" && input.Credential.Password != "" {
		fillLogin(page.WebSocketDebuggerURL, input.Credential.Email, input.Credential.Password)
	}
	if os.Getenv("WEBWRIGHT_MANUAL_LOGIN") == "true" {
		waitForPortalLogin(cdpURL, page.ID)
		fmt.Print(`{"transactions":[]}` + "\n")
		return
	}
	waitForPortalLogin(cdpURL, page.ID)
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
	var last string
	for attempt := 0; attempt < 5; attempt++ {
		response, err := client.Get(strings.TrimRight(cdpURL, "/") + "/json/list")
		if err == nil && response.StatusCode == http.StatusOK {
			var result []pageTarget
			err = json.NewDecoder(response.Body).Decode(&result)
			response.Body.Close()
			if err == nil {
				return result
			}
		}
		if response != nil {
			response.Body.Close()
			last = response.Status
		} else if err != nil {
			last = err.Error()
		}
		time.Sleep(250 * time.Millisecond)
	}
	fail("cannot inspect Neko browser: " + last)
	return nil
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

func fillLogin(wsURL, email, password string) {
	conn, _, _, err := ws.Dial(context.Background(), wsURL)
	if err != nil {
		fail("cannot control the Neko portal tab: " + err.Error())
	}
	defer conn.Close()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		expression := fmt.Sprintf(`(() => {
			const set = (el, value) => { if (!el) return false; const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set; if (setter) setter.call(el, value); else el.value = value; for (const type of ['input','change','blur']) el.dispatchEvent(new Event(type, {bubbles:true})); return true; };
			const email = document.querySelector('input[type=email], input[name*=email i], input[name*=username i], input[type=text]');
			const password = document.querySelector('input[type=password]');
			const ready = set(email, %s) && set(password, %s);
			if (ready) { const submit = document.querySelector('button[type=submit], input[type=submit], button.login, button'); if (submit && !submit.disabled) submit.click(); }
			return ready;
		})()`, jsString(email), jsString(password))
		command, _ := json.Marshal(map[string]any{"id": 1, "method": "Runtime.evaluate", "params": map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}})
		if err := wsutil.WriteClientText(conn, command); err != nil {
			fail("cannot fill portal credentials: " + err.Error())
		}
		if _, _, err := wsutil.ReadClientData(conn); err != nil {
			fail("cannot verify portal form: " + err.Error())
		}
		return
		// The page can still be loading; retry the same persistent tab.
		time.Sleep(500 * time.Millisecond)
	}
	fail("portal login form did not become ready in Neko")
}

func waitForPortalLogin(cdpURL, id string) {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		page := pageByID(cdpURL, id)
		if !isLoginPage(page.URL) {
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
