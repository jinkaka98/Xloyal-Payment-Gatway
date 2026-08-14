package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
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

type transaction struct {
	Reference string `json:"reference"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
	PaidAt    string `json:"paid_at"`
}

type portalState struct {
	Login         bool `json:"login"`
	Authenticated bool `json:"authenticated"`
	HistoryReady  bool `json:"history_ready"`
}

type historyRefreshState struct {
	Status     string `json:"status"`
	BeforeDraw int    `json:"before_draw"`
}

type cdpClient struct {
	conn *websocket.Conn
	next int
}

func main() {
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil || input.MerchantID == "" {
		fail("checker input requires merchant_id")
	}
	run(input, runX11Helper)
}

func run(input request, fallback func(request)) {
	cdpURL := env("NEKO_CDP_URL", "http://neko:9222")
	portalURL := env("WEBWRIGHT_PORTAL_URL", "https://merchant.qris.interactive.co.id")
	page, err := portalPage(cdpURL, portalURL)
	if err != nil {
		fallback(input)
		return
	}
	client, err := newCDPClient(page.WebSocketDebuggerURL)
	if err != nil {
		fallback(input)
		return
	}
	defer client.Close()
	if isLoginPage(page.URL) && input.Credential != nil && input.Credential.Email != "" && input.Credential.Password != "" {
		if err := fillLogin(client, input.Credential.Email, input.Credential.Password); err != nil {
			fallback(input)
			return
		}
	}
	if err := waitForPortalLogin(client, portalURL); err != nil {
		fallback(input)
		return
	}
	transactions, err := extractTransactions(client)
	if err != nil {
		fallback(input)
		return
	}
	output, _ := json.Marshal(map[string]any{"transactions": transactions})
	fmt.Println(string(output))
}

func runX11Helper(input request) {
	helperURL := env("NEKO_HELPER_URL", "http://neko:9224/sync")
	body, _ := json.Marshal(input)
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, helperURL, strings.NewReader(string(body)))
	if err != nil {
		fail("cannot prepare Neko X11 helper: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{}).Do(req)
	if err != nil {
		fail("cannot reach Neko X11 helper: " + err.Error())
	}
	defer response.Body.Close()
	var payload struct {
		Transactions []transaction `json:"transactions"`
		Error        string        `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		fail("cannot decode Neko X11 helper response: " + err.Error())
	}
	if response.StatusCode != http.StatusOK {
		fail("Neko X11 helper failed: " + payload.Error)
	}
	output, _ := json.Marshal(map[string]any{"transactions": payload.Transactions})
	fmt.Println(string(output))
}

func extractTransactions(client *cdpClient) ([]transaction, error) {
	// The portal's daterangepicker owns the input value. Update the widget,
	// request the current month with the supported maximum, then click its
	// real submit button so the same request path as a human operator is used.
	now := time.Now().In(time.FixedZone("WIB", 7*60*60))
	start, end := currentMonthRange(now)
	filter := fmt.Sprintf(`(() => {
		const input=document.querySelector('#range-transactions'), limit=document.querySelector('#limit-transactions'), submit=document.querySelector('#getResults');
		const table=window.jQuery?.fn?.dataTable?.isDataTable('#transaction-summary') ? window.jQuery('#transaction-summary').DataTable() : null;
		const settings=table?.settings?.()[0];
		if (!input || !limit || !submit || !settings) return JSON.stringify({status:'missing_history_controls',before_draw:-1});
		const beforeDraw=settings.iDraw;
		const picker=window.jQuery?.(input).data('daterangepicker');
		if (picker) { picker.setStartDate(%s); picker.setEndDate(%s); }
		input.value=%s+' - '+%s; input.dispatchEvent(new Event('change',{bubbles:true}));
		limit.value='300'; limit.dispatchEvent(new Event('change',{bubbles:true})); submit.click(); return JSON.stringify({status:'submitted',before_draw:beforeDraw});
	})()`, jsString(start), jsString(end), jsString(start), jsString(end))
	fmt.Fprintln(os.Stderr, "checker: applying current-month history filter")
	filterResult, err := client.Evaluate(filter)
	if err != nil {
		return nil, err
	}
	filterRaw, ok := filterResult.(string)
	if !ok {
		return nil, fmt.Errorf("history filter returned %T", filterResult)
	}
	refreshState, err := decodeHistoryRefreshState(filterRaw)
	if err != nil || refreshState.Status != "submitted" {
		return nil, fmt.Errorf("authenticated portal history controls were not found")
	}
	deadline := time.Now().Add(30 * time.Second)
	refreshComplete := false
	for time.Now().Before(deadline) {
		value, err := client.Evaluate(historyRefreshCompleteExpression(refreshState.BeforeDraw))
		if err == nil && value == true {
			refreshComplete = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !refreshComplete {
		return nil, fmt.Errorf("portal history request did not complete before timeout")
	}
	expression := `(() => JSON.stringify([...document.querySelectorAll('#transaction-summary tbody tr')].map(row=>{
		const c=[...row.cells].map(cell=>(cell.textContent||'').trim());
		const match=(c[0]||'').match(/(\d{2})\/(\d{2})\/(\d{4})\s+(\d{2}):(\d{2}):(\d{2})/);
		const status=(c[4]||'').toLowerCase();
		return {reference:[c[7],c[9],c[10]].filter(Boolean).join(' '),amount:Number.parseInt((c[2]||c[1]||'').replace(/[^0-9-]/g,''),10)||0,status:status.includes('sukses')?'paid':status.includes('gagal')?'failed':status,paid_at:match?(match[3]+'-'+match[2]+'-'+match[1]+'T'+match[4]+':'+match[5]+':'+match[6]+'+07:00'):''};
	}).filter(x=>x.reference&&x.amount&&x.paid_at)))()`
	value, err := client.Evaluate(expression)
	if err != nil {
		return nil, err
	}
	var out []transaction
	raw, _ := value.(string)
	if raw == "" {
		return []transaction{}, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func historyRefreshCompleteExpression(beforeDraw int) string {
	return fmt.Sprintf(`(() => { const table=window.jQuery?.fn?.dataTable?.isDataTable('#transaction-summary') ? window.jQuery('#transaction-summary').DataTable() : null; const settings=table?.settings?.()[0]; const processing=document.querySelector('#transaction-summary_processing'); const ready=!processing || !processing.offsetParent; return Boolean(settings && settings.iDraw > %d && ready && (!settings.jqXHR || settings.jqXHR.readyState===4)); })()`, beforeDraw)
}

func decodeHistoryRefreshState(value any) (historyRefreshState, error) {
	raw, ok := value.(string)
	if !ok {
		return historyRefreshState{}, fmt.Errorf("history refresh value has type %T", value)
	}
	var state historyRefreshState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return historyRefreshState{}, err
	}
	return state, nil
}

func currentMonthRange(now time.Time) (string, string) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return start.Format("02/01/2006"), now.Format("02/01/2006")
}

func newCDPClient(wsURL string) (*cdpClient, error) {
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}
	endpoint := parsed.Host
	parsed.Host = "127.0.0.1:9223"
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, NetDialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, endpoint)
	}}
	conn, _, err := dialer.DialContext(context.Background(), parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	return &cdpClient{conn: conn}, nil
}

func (c *cdpClient) Close() error { return c.conn.Close() }

func (c *cdpClient) call(method string, params map[string]any) (json.RawMessage, error) {
	c.next++
	id := c.next
	_ = c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if err := c.conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(payload, &response) != nil || response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("%s", response.Error.Message)
		}
		return response.Result, nil
	}
}

func (c *cdpClient) Evaluate(expression string) (any, error) {
	raw, err := c.call("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true})
	if err != nil {
		return nil, err
	}
	var result struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result.Exception) != 0 && string(result.Exception) != "null" {
		return nil, fmt.Errorf("portal script evaluation failed")
	}
	return result.Result.Value, nil
}

func (c *cdpClient) Navigate(location string) error {
	_, err := c.call("Page.navigate", map[string]any{"url": location})
	return err
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func pages(cdpURL string) ([]pageTarget, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	var last string
	for attempt := 0; attempt < 5; attempt++ {
		request, _ := http.NewRequest(http.MethodGet, strings.TrimRight(cdpURL, "/")+"/json/list", nil)
		request.Host = "127.0.0.1:9223"
		response, err := client.Do(request)
		if err == nil && response.StatusCode == http.StatusOK {
			var result []pageTarget
			err = json.NewDecoder(response.Body).Decode(&result)
			response.Body.Close()
			if err == nil {
				for index := range result {
					result[index].WebSocketDebuggerURL = normalizeWebSocketURL(result[index].WebSocketDebuggerURL, cdpURL)
				}
				return result, nil
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
	return nil, fmt.Errorf("cannot inspect Neko browser: %s", last)
}

func portalPage(cdpURL, portalURL string) (pageTarget, error) {
	candidates, err := pages(cdpURL)
	if err != nil {
		return pageTarget{}, err
	}
	if page, ok := selectPortalPage(candidates); ok {
		return page, nil
	}
	request, err := http.NewRequest(http.MethodPut, strings.TrimRight(cdpURL, "/")+"/json/new?"+url.QueryEscape(portalURL), nil)
	if err != nil {
		return pageTarget{}, fmt.Errorf("cannot prepare portal browser: %w", err)
	}
	request.Host = "127.0.0.1:9223"
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return pageTarget{}, fmt.Errorf("cannot open portal in Neko: %w", err)
	}
	defer response.Body.Close()
	var page pageTarget
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&page) != nil {
		return pageTarget{}, fmt.Errorf("cannot open portal in Neko: %s", response.Status)
	}
	page.WebSocketDebuggerURL = normalizeWebSocketURL(page.WebSocketDebuggerURL, cdpURL)
	return page, nil
}

func normalizeWebSocketURL(wsURL, cdpURL string) string {
	endpoint := strings.TrimPrefix(strings.TrimPrefix(cdpURL, "http://"), "https://")
	return strings.Replace(wsURL, "127.0.0.1:9223", endpoint, 1)
}

func selectPortalPage(candidates []pageTarget) (pageTarget, bool) {
	var loginPage pageTarget
	for _, page := range candidates {
		if page.Type != "page" || !strings.Contains(page.URL, "merchant.qris.interactive.co.id") || strings.Contains(page.URL, "about:blank") {
			continue
		}
		if strings.Contains(page.URL, "/v2/m/kontenr.php") {
			return page, true
		}
		if loginPage.ID == "" {
			loginPage = page
		}
	}
	if loginPage.ID != "" {
		return loginPage, true
	}
	return pageTarget{}, false
}

func pageByID(cdpURL, id string) pageTarget {
	candidates, err := pages(cdpURL)
	if err != nil {
		fail(err.Error())
	}
	for _, page := range candidates {
		if page.ID == id {
			return page
		}
	}
	fail("Neko portal tab was closed")
	return pageTarget{}
}

func fillLogin(client *cdpClient, email, password string) error {
	var diagnostic any
	var lastErr error
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		expression := fmt.Sprintf(`(() => {
			const visible = el => { const s=getComputedStyle(el); return s.display!=='none' && s.visibility!=='hidden' && el.getBoundingClientRect().width>0; };
			const set = (el, value) => { if (!el) return 0; const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set; if (setter) setter.call(el, value); else el.value = value; el.dispatchEvent(new Event('input', {bubbles:true})); return el.value.length; };
			const email = [...document.querySelectorAll('input')].find(el => visible(el) && (el.type==='email' || /email|user/i.test(el.name+el.id+el.placeholder))) || [...document.querySelectorAll('input')].find(el => visible(el) && el.type==='text');
			const password = [...document.querySelectorAll('input[type=password]')].find(visible);
			return JSON.stringify({email:set(email,%s),password:set(password,%s)});
		})()`, jsString(email), jsString(password))
		value, err := client.Evaluate(expression)
		lastErr = err
		if err != nil && time.Since(deadline.Add(-30*time.Second)) < 2*time.Second {
			fmt.Fprintf(os.Stderr, "checker: fill evaluate error: %v\n", err)
		}
		if err == nil {
			state, decodeErr := decodeFillState(value)
			if decodeErr == nil && state.Email > 0 && state.Password > 0 {
				return nil
			}
		}
		diagnostic, _ = client.Evaluate(`JSON.stringify({url:location.href,ready:document.readyState,inputs:[...document.querySelectorAll('input')].map(x=>({type:x.type,name:x.name,id:x.id,placeholder:x.placeholder})),frames:[...document.querySelectorAll('iframe')].map(x=>({src:x.src,id:x.id,name:x.name}))})`)
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("portal login form did not become ready in Neko: evaluate=%v dom=%v", lastErr, diagnostic)
}

type fillState struct {
	Email    int `json:"email"`
	Password int `json:"password"`
}

func decodeFillState(value any) (fillState, error) {
	raw, ok := value.(string)
	if !ok {
		return fillState{}, fmt.Errorf("fill response has type %T", value)
	}
	var state fillState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return fillState{}, err
	}
	return state, nil
}

func waitForPortalLogin(client *cdpClient, portalURL string) error {
	fmt.Fprintln(os.Stderr, "checker: validating authenticated history page")
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		value, err := client.Evaluate(`(() => { const login=Boolean(document.querySelector('input[type=password]')); const dashboard=location.hostname==='merchant.qris.interactive.co.id' && location.pathname.includes('/v2/m/kontenr.php'); const historyReady=Boolean(document.querySelector('#transaction-summary') && document.querySelector('#range-transactions')); const authenticated=dashboard || Boolean(document.querySelector('a[href*="historytrx.php"], #transaction-summary, #range-transactions')); return JSON.stringify({login,authenticated,history_ready:historyReady}); })()`)
		if err != nil {
			return fmt.Errorf("portal CDP validation failed: %w", err)
		}
		if err == nil {
			state, stateErr := decodePortalState(value)
			if stateErr == nil && state.Authenticated && !state.Login {
				if !requiresHistoryNavigation(state) {
					return nil
				}
				historyURL := strings.TrimRight(portalURL, "/") + "/v2/m/kontenr.php?idir=pages/historytrx.php"
				if err = client.Navigate(historyURL); err == nil {
					for waitUntil := time.Now().Add(30 * time.Second); time.Now().Before(waitUntil); time.Sleep(500 * time.Millisecond) {
						ready, readyErr := client.Evaluate(`Boolean(document.querySelector('#transaction-summary') && document.querySelector('#range-transactions'))`)
						if readyErr == nil && ready == true {
							return nil
						}
					}
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("manual login did not complete in Neko before timeout")
}

func requiresHistoryNavigation(state portalState) bool {
	return state.Authenticated && !state.Login && !state.HistoryReady
}

func decodePortalState(value any) (portalState, error) {
	raw, ok := value.(string)
	if !ok {
		return portalState{}, fmt.Errorf("portal state response has type %T", value)
	}
	var state portalState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return portalState{}, err
	}
	return state, nil
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
