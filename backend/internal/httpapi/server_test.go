package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	qrisservice "xloyal/backend/internal/qris"
	"xloyal/backend/internal/security"
	"xloyal/backend/internal/store"
)

func TestGitHubWebhookRequiresValidSignatureAndSignalsMainCommit(t *testing.T) {
	signal := t.TempDir() + "/deploy-request"
	h := Server{WebhookSecret: "webhook-secret", WebhookSignalPath: signal}.Handler()
	body := []byte(`{"ref":"refs/heads/main","after":"706a74b6f0c21d5a575de8e9a2884f6b60738188"}`)

	invalid := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPost, "/internal/github/webhook", bytes.NewReader(body))
	invalidRequest.Header.Set("X-GitHub-Event", "push")
	invalidRequest.Header.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64))
	h.ServeHTTP(invalid, invalidRequest)
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("invalid signature code=%d body=%s", invalid.Code, invalid.Body.String())
	}

	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = mac.Write(body)
	valid := httptest.NewRecorder()
	validRequest := httptest.NewRequest(http.MethodPost, "/internal/github/webhook", bytes.NewReader(body))
	validRequest.Header.Set("X-GitHub-Event", "push")
	validRequest.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
	h.ServeHTTP(valid, validRequest)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid signature code=%d body=%s", valid.Code, valid.Body.String())
	}
	written, err := os.ReadFile(signal)
	if err != nil || string(written) != "706a74b6f0c21d5a575de8e9a2884f6b60738188\n" {
		t.Fatalf("signal=%q err=%v", written, err)
	}
}

func TestAdminPaymentThemeLifecycleIsolationAndRBAC(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	server := Server{Repo: repo, AdminTokens: map[string]string{"viewer": "viewer", "operator": "operator", "root": "super_admin"}}.Handler()
	config := `{"schema_version":1,"template_key":"modern","colors":{"primary":"#1A5C55","background":"#F4F6F2","surface":"#FFFFFF","text":"#18231F","muted":"#68766F","success":"#16805B","danger":"#B44444"}}`
	call := func(token, method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		server.ServeHTTP(w, r)
		return w
	}

	viewerCreate := call("viewer", http.MethodPost, "/admin/payment-themes", `{"tenant_id":"tenant-a","name":"Blue","config":`+config+`}`)
	if viewerCreate.Code != http.StatusForbidden {
		t.Fatalf("viewer create=%d", viewerCreate.Code)
	}
	created := call("operator", http.MethodPost, "/admin/payment-themes", `{"tenant_id":"tenant-a","name":"Blue","config":`+config+`}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create=%d body=%s", created.Code, created.Body.String())
	}
	var theme themeDTO
	if err := json.Unmarshal(created.Body.Bytes(), &theme); err != nil {
		t.Fatal(err)
	}
	if theme.Status != domain.ThemeDraft || theme.Version != 0 {
		t.Fatalf("created theme=%+v", theme)
	}
	if cross := call("viewer", http.MethodGet, "/admin/payment-themes/"+theme.ID+"?tenant_id=tenant-b", ""); cross.Code != http.StatusNotFound {
		t.Fatalf("cross tenant=%d", cross.Code)
	}
	if bad := call("operator", http.MethodPut, "/admin/payment-themes/"+theme.ID+"?tenant_id=tenant-a", `{"name":"Blue","config":{"template_key":"modern","unexpected":"script"}}`); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid config=%d", bad.Code)
	}
	published := call("operator", http.MethodPost, "/admin/payment-themes/"+theme.ID+"/publish?tenant_id=tenant-a", "")
	if published.Code != http.StatusOK {
		t.Fatalf("publish=%d body=%s", published.Code, published.Body.String())
	}
	first, err := repo.PublishedPaymentThemeVersion(ctx, "tenant-a", theme.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	updatedConfig := strings.Replace(config, "#1A5C55", "#185FA5", 1)
	updated := call("operator", http.MethodPut, "/admin/payment-themes/"+theme.ID+"?tenant_id=tenant-a", `{"name":"Blue v2","config":`+updatedConfig+`}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update published draft=%d body=%s", updated.Code, updated.Body.String())
	}
	if republish := call("operator", http.MethodPost, "/admin/payment-themes/"+theme.ID+"/publish?tenant_id=tenant-a", ""); republish.Code != http.StatusOK {
		t.Fatalf("republish=%d", republish.Code)
	}
	if string(first.Config) != config {
		t.Fatalf("published version was mutated: %s", first.Config)
	}
	if current, err := repo.PublishedPaymentThemeVersion(ctx, "tenant-a", theme.ID, 2); err != nil || string(current.Config) != updatedConfig {
		t.Fatalf("version two=%+v err=%v", current, err)
	}
	if defaultDraft := call("operator", http.MethodPost, "/admin/payment-themes/"+theme.ID+"/set-default?tenant_id=tenant-a", ""); defaultDraft.Code != http.StatusOK {
		t.Fatalf("set default=%d", defaultDraft.Code)
	}
	if archived := call("operator", http.MethodPost, "/admin/payment-themes/"+theme.ID+"/archive?tenant_id=tenant-a", ""); archived.Code != http.StatusConflict {
		t.Fatalf("archive default=%d", archived.Code)
	}
	audit, err := repo.ListAudit(ctx, "tenant-a", 100)
	if err != nil || len(audit) < 5 {
		t.Fatalf("theme audit count=%d err=%v", len(audit), err)
	}
}

func TestAdminPaymentThemeGlobalLibraryDoesNotRequireTenant(t *testing.T) {
	repo := store.NewMemory()
	server := Server{Repo: repo, AdminTokens: map[string]string{"operator": "operator"}}.Handler()
	body := `{"name":"Global Checkout","config":{"schema_version":1,"template_key":"modern","tenant_branding":{"display_name":"Global Merchant","primary_color":"#1A5C55"}}}`
	req := httptest.NewRequest(http.MethodPost, "/admin/payment-themes", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer operator")
	req.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	server.ServeHTTP(created, req)
	if created.Code != http.StatusCreated {
		t.Fatalf("global create=%d body=%s", created.Code, created.Body.String())
	}
	var theme themeDTO
	if err := json.Unmarshal(created.Body.Bytes(), &theme); err != nil {
		t.Fatal(err)
	}
	if theme.TenantID != "" {
		t.Fatalf("global theme tenant=%q", theme.TenantID)
	}
	listReq := httptest.NewRequest(http.MethodGet, "/admin/payment-themes", nil)
	listReq.Header.Set("Authorization", "Bearer operator")
	listed := httptest.NewRecorder()
	server.ServeHTTP(listed, listReq)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), theme.ID) {
		t.Fatalf("global list=%d body=%s", listed.Code, listed.Body.String())
	}
}

type provider struct{}

func (provider) CreatePayment(context.Context, domain.CreatePaymentRequest) (domain.CreatePaymentResult, error) {
	return domain.CreatePaymentResult{ProviderReference: "ref", QRPayload: "000201010212fixture"}, nil
}
func (provider) CheckPayment(context.Context, domain.CheckPaymentRequest) (domain.CheckPaymentResult, error) {
	return domain.CheckPaymentResult{Status: domain.InvoicePaid}, nil
}
func (provider) Health(context.Context) error { return nil }
func TestAPIAuthIsolationIdempotencyAndQR(t *testing.T) {
	r := store.NewMemory()
	ctx := context.Background()
	r.CreateTenant(ctx, domain.Tenant{ID: "t1", APIKeyHash: security.HashSecret("key1"), Active: true})
	r.CreateTenant(ctx, domain.Tenant{ID: "t2", APIKeyHash: security.HashSecret("key2"), Active: true})
	r.CreateMerchantAccount(ctx, domain.MerchantAccount{ID: "m1", TenantID: "t1", Active: true})
	c, _ := security.NewCipher(bytes.Repeat([]byte{1}, 32))
	g := gateway.Service{Repo: r, Provider: func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error) { return provider{}, nil }}
	h := Server{Repo: r, Gateway: g, Cipher: c, AdminTokens: map[string]string{"admin": "super_admin"}}.Handler()
	body := []byte(`{"merchant_account_id":"m1","idempotency_key":"same","amount":1000}`)
	call := func(key string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/tenants/t1/invoices", bytes.NewReader(body))
		req.Header.Set("X-API-Key", key)
		h.ServeHTTP(w, req)
		return w
	}
	a := call("key1")
	b := call("key1")
	if a.Code != 201 || b.Code != 200 {
		t.Fatalf("codes %d %d", a.Code, b.Code)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/tenants/t1/invoices", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "key2")
	h.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("cross tenant code %d", w.Code)
	}
}

func TestTenantAPIOriginPolicy(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-production", SiteURL: "https://Shop.Example:443/account", APIKeyHash: security.HashSecret("production-key"), Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-http", SiteURL: "http://LOCALHOST:80/app", APIKeyHash: security.HashSecret("http-key"), Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-sandbox", SiteURL: "https://sandbox.example", SandboxMode: true, APIKeyHash: security.HashSecret("sandbox-key"), Active: true})
	h := Server{Repo: repo}.Handler()

	tests := []struct {
		name        string
		tenantID    string
		key         string
		origin      string
		wantCode    int
		allowOrigin string
	}{
		{name: "production matching origin", tenantID: "tenant-production", key: "production-key", origin: "https://shop.example", wantCode: http.StatusOK, allowOrigin: "https://shop.example"},
		{name: "production HTTP default port", tenantID: "tenant-http", key: "http-key", origin: "http://localhost", wantCode: http.StatusOK, allowOrigin: "http://localhost"},
		{name: "production foreign origin", tenantID: "tenant-production", key: "production-key", origin: "https://foreign.example", wantCode: http.StatusForbidden},
		{name: "sandbox foreign origin", tenantID: "tenant-sandbox", key: "sandbox-key", origin: "http://localhost:3000", wantCode: http.StatusOK, allowOrigin: "http://localhost:3000"},
		{name: "server request without origin", tenantID: "tenant-production", key: "production-key", wantCode: http.StatusOK},
		{name: "sandbox invalid API key", tenantID: "tenant-sandbox", key: "wrong-key", origin: "http://localhost:3000", wantCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/tenants/"+tt.tenantID+"/transactions", nil)
			req.Header.Set("X-API-Key", tt.key)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			h.ServeHTTP(w, req)
			if w.Code != tt.wantCode {
				t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.allowOrigin {
				t.Fatalf("allow origin=%q want=%q", got, tt.allowOrigin)
			}
			if tt.origin != "" && !strings.Contains(w.Header().Get("Vary"), "Origin") {
				t.Fatalf("Vary=%q", w.Header().Get("Vary"))
			}
		})
	}

	preflight := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/v1/tenants/tenant-sandbox/transactions/qris", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "X-API-Key, Idempotency-Key, Content-Type")
	h.ServeHTTP(preflight, req)
	if preflight.Code != http.StatusNoContent || preflight.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" || preflight.Header().Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" || preflight.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-API-Key, Idempotency-Key" {
		t.Fatalf("preflight code=%d headers=%v body=%s", preflight.Code, preflight.Header(), preflight.Body.String())
	}
}
func TestAdminRBAC(t *testing.T) {
	r := store.NewMemory()
	c, _ := security.NewCipher(bytes.Repeat([]byte{1}, 32))
	h := Server{Repo: r, Cipher: c, AdminTokens: map[string]string{"view": "viewer"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", bytes.NewBufferString(`{"id":"t","api_key":"k"}`))
	req.Header.Set("Authorization", "Bearer view")
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("viewer wrote tenant: %d", w.Code)
	}
}

func TestHARImportCreatesReconnectRequiredBrowserConnection(t *testing.T) {
	repo := store.NewMemory()
	c, _ := security.NewCipher(bytes.Repeat([]byte{1}, 32))
	h := Server{Repo: repo, Cipher: c, AdminTokens: map[string]string{"admin": "operator"}}.Handler()

	har := `{"log":{"pages":[{"title":"history"}],"entries":[{"request":{"url":"https://merchant.qris.interactive.co.id/v2/m/kontenr.php?idir=pages/historytrx.php"}}]}}`
	imported := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/merchant-connections/har", strings.NewReader(har))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(imported, req)
	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), `"merchant_id":"interactive-browser"`) || !strings.Contains(imported.Body.String(), `"session_required":true`) {
		t.Fatalf("import code=%d body=%s", imported.Code, imported.Body.String())
	}
	connection, err := repo.MerchantConnection(context.Background(), "interactive-browser")
	if err != nil || connection.Status != domain.ConnectionReconnectRequired {
		t.Fatalf("connection=%+v err=%v", connection, err)
	}
}

func TestSessionImportAcceptsChromeCookieExport(t *testing.T) {
	repo := store.NewMemory()
	c, _ := security.NewCipher(bytes.Repeat([]byte{1}, 32))
	repo.CreateMerchantID(context.Background(), domain.MerchantID{ID: "merchant_1"})
	repo.UpsertMerchantConnection(context.Background(), domain.MerchantConnection{MerchantID: "merchant_1", Status: domain.ConnectionReconnectRequired})
	h := Server{Repo: repo, Cipher: c, AdminTokens: map[string]string{"admin": "operator"}}.Handler()
	body := `[{"domain":"merchant.qris.interactive.co.id","hostOnly":true,"httpOnly":true,"name":"PHPSESSID","path":"/","sameSite":"lax","secure":true,"session":true,"storeId":null,"value":"session-value"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/merchant-ids/merchant_1/connection/session", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"connected"`) {
		t.Fatalf("import code=%d body=%s", w.Code, w.Body.String())
	}
	connection, _ := repo.MerchantConnection(context.Background(), "merchant_1")
	if connection.SessionCiphertext == "" || connection.Status != domain.ConnectionConnected {
		t.Fatalf("connection=%+v", connection)
	}
}

func TestReconnectRequiredCanQueueAutomaticConnection(t *testing.T) {
	repo := store.NewMemory()
	c, _ := security.NewCipher(bytes.Repeat([]byte{1}, 32))
	now := time.Now().UTC()
	repo.UpsertMerchantConnection(context.Background(), domain.MerchantConnection{MerchantID: "merchant_1", Status: domain.ConnectionReconnectRequired, LastError: "old error", UpdatedAt: now})
	h := Server{Repo: repo, Cipher: c, AdminTokens: map[string]string{"admin": "operator"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/merchant-ids/merchant_1/sync", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("queue code=%d body=%s", w.Code, w.Body.String())
	}
	connection, _ := repo.MerchantConnection(context.Background(), "merchant_1")
	if connection.Status != domain.ConnectionReconnectRequired || connection.LastError != "Browser connection queued" || connection.UpdatedAt.Before(now) {
		t.Fatalf("connection=%+v", connection)
	}
	job, claimed, err := repo.ClaimBrowserJob(context.Background(), "worker", time.Now().UTC(), time.Minute)
	if err != nil || !claimed || job.Kind != "merchant_sync" || job.MerchantID != "merchant_1" {
		t.Fatalf("job=%+v claimed=%v err=%v", job, claimed, err)
	}
}

func TestManualLoginQueuesWorkerJobWithoutRunningBrowserInAPI(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant_manual", Status: domain.ConnectionReconnectRequired, LastSyncedAt: func() *time.Time { value := time.Now().UTC(); return &value }(), UpdatedAt: time.Now().UTC()})
	called := false
	manualLogin := func(context.Context, domain.MerchantConnection) error {
		called = true
		return nil
	}
	h := Server{Repo: repo, ManualLogin: manualLogin, AdminTokens: map[string]string{"admin": "operator"}}.Handler()
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/merchant-ids/merchant_manual/connection/manual-login", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(response, req)
	if response.Code != http.StatusAccepted {
		t.Fatalf("manual login code=%d body=%s", response.Code, response.Body.String())
	}
	if called {
		t.Fatal("API executed browser login instead of queueing it")
	}
	connection, _ := repo.MerchantConnection(ctx, "merchant_manual")
	if connection.LastError != "Manual browser login queued" {
		t.Fatalf("queued connection=%+v", connection)
	}
	job, claimed, err := repo.ClaimBrowserJob(ctx, "worker", time.Now().UTC(), time.Minute)
	if err != nil || !claimed || job.Kind != "manual_login" || job.MerchantID != "merchant_manual" {
		t.Fatalf("queued browser job=%+v claimed=%v err=%v", job, claimed, err)
	}
}

func TestUpdateMerchantBrowserCredentialEncryptsAndQueuesReconnect(t *testing.T) {
	repo := store.NewMemory()
	cipher, _ := security.NewCipher(bytes.Repeat([]byte{3}, 32))
	repo.UpsertMerchantConnection(context.Background(), domain.MerchantConnection{MerchantID: "merchant_credential", Status: domain.ConnectionConnected, UpdatedAt: time.Now().UTC()})
	h := Server{Repo: repo, Cipher: cipher, AdminTokens: map[string]string{"admin": "operator"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/merchant-ids/merchant_credential/connection/credentials", strings.NewReader(`{"email":"merchant@example.com","password":"valid-password"}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	connection, _ := repo.MerchantConnection(context.Background(), "merchant_credential")
	if connection.Status != domain.ConnectionReconnectRequired || connection.LastError != "Browser connection queued" || !connection.UpdatedAt.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("connection=%+v", connection)
	}
	plain, err := cipher.Decrypt(connection.BrowserCredentialCiphertext)
	if err != nil || !strings.Contains(string(plain), `"email":"merchant@example.com"`) || strings.Contains(connection.BrowserCredentialCiphertext, "valid-password") {
		t.Fatalf("credential encryption failed")
	}
}

func TestCreateTenantGeneratesCredentialsAndPersistsConnectivity(t *testing.T) {
	repo := store.NewMemory()
	cipher, _ := security.NewCipher(bytes.Repeat([]byte{4}, 32))
	repo.CreateMerchantID(context.Background(), domain.MerchantID{ID: "interactive-browser", Name: "InterActive QRIS browser", Active: true})
	h := Server{Repo: repo, Cipher: cipher, AdminTokens: map[string]string{"admin": "super_admin"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/tenants", strings.NewReader(`{"name":"Website utama","merchant_id":"interactive-browser","site_url":"https://shop.example","callback_url":"https://shop.example/qris/callback","webhook_url":"https://shop.example/webhooks/qris","sandbox_mode":true,"use_unique_amount_code":true}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create code=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Tenant            domain.Tenant `json:"tenant"`
		APIKey            string        `json:"api_key"`
		APIKeyVisibleOnce bool          `json:"api_key_visible_once"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(response.Tenant.ID, "tenant_") || !strings.HasPrefix(response.APIKey, "xl_live_") || !response.APIKeyVisibleOnce {
		t.Fatalf("generated response=%+v", response)
	}
	if response.Tenant.MerchantID != "interactive-browser" || response.Tenant.SiteURL != "https://shop.example" || response.Tenant.CallbackURL == "" || response.Tenant.WebhookURL == "" || !response.Tenant.SandboxMode || !response.Tenant.UseUniqueAmountCode {
		t.Fatalf("tenant=%+v", response.Tenant)
	}
	authenticated, err := repo.TenantByAPIKey(context.Background(), security.HashSecret(response.APIKey))
	if err != nil || authenticated.ID != response.Tenant.ID {
		t.Fatalf("generated API key did not authenticate: tenant=%+v err=%v", authenticated, err)
	}
	if strings.Contains(w.Body.String(), "api_key_hash") {
		t.Fatalf("API key hash leaked: %s", w.Body.String())
	}
	stored, err := repo.Tenant(context.Background(), response.Tenant.ID)
	if err != nil || stored.APIKeyCiphertext == "" || stored.APIKeyHash == "" {
		t.Fatalf("tenant credential was not persisted: tenant=%+v err=%v", stored, err)
	}
	plain, err := cipher.Decrypt(stored.APIKeyCiphertext)
	if err != nil || string(plain) != response.APIKey {
		t.Fatal("persisted tenant credential did not decrypt to the generated API key")
	}
	if strings.Contains(w.Body.String(), stored.APIKeyCiphertext) {
		t.Fatal("tenant credential ciphertext leaked in create response")
	}
}

func TestTenantCredentialRevealAndRotateAreSuperAdminOnly(t *testing.T) {
	repo := store.NewMemory()
	cipher, _ := security.NewCipher(bytes.Repeat([]byte{5}, 32))
	oldKey := "xl_live_existing"
	ciphertext, _ := cipher.Encrypt([]byte(oldKey))
	repo.CreateTenant(context.Background(), domain.Tenant{ID: "tenant-secret", Name: "Secret", APIKeyHash: security.HashSecret(oldKey), APIKeyCiphertext: ciphertext, Active: true, CreatedAt: time.Now().UTC()})
	h := Server{Repo: repo, Cipher: cipher, AdminTokens: map[string]string{"view": "viewer", "operate": "operator", "admin": "super_admin"}}.Handler()
	listed := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	listRequest.Header.Set("Authorization", "Bearer view")
	h.ServeHTTP(listed, listRequest)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"api_key_recoverable":true`) || strings.Contains(listed.Body.String(), oldKey) || strings.Contains(listed.Body.String(), ciphertext) {
		t.Fatalf("tenant list leaked credential or omitted recovery state: code=%d body=%s", listed.Code, listed.Body.String())
	}

	for _, token := range []string{"view", "operate"} {
		for method, path := range map[string]string{
			http.MethodGet:  "/admin/tenants/tenant-secret/credentials",
			http.MethodPost: "/admin/tenants/tenant-secret/credentials/rotate",
		} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			h.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), oldKey) {
				t.Fatalf("role %s %s code=%d body=%s", token, path, w.Code, w.Body.String())
			}
		}
	}

	revealed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant-secret/credentials", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(revealed, req)
	if revealed.Code != http.StatusOK || !strings.Contains(revealed.Body.String(), `"api_key":"`+oldKey+`"`) {
		t.Fatalf("reveal code=%d body=%s", revealed.Code, revealed.Body.String())
	}

	rotated := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/tenants/tenant-secret/credentials/rotate", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(rotated, req)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate code=%d body=%s", rotated.Code, rotated.Body.String())
	}
	var response struct {
		TenantID string `json:"tenant_id"`
		APIKey   string `json:"api_key"`
	}
	if err := json.Unmarshal(rotated.Body.Bytes(), &response); err != nil || response.TenantID != "tenant-secret" || !strings.HasPrefix(response.APIKey, "xl_live_") || response.APIKey == oldKey {
		t.Fatalf("rotated response=%+v err=%v", response, err)
	}
	if _, err := repo.TenantByAPIKey(context.Background(), security.HashSecret(oldKey)); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old API key still authenticates: %v", err)
	}
	if tenant, err := repo.TenantByAPIKey(context.Background(), security.HashSecret(response.APIKey)); err != nil || tenant.ID != "tenant-secret" {
		t.Fatalf("new API key does not authenticate: tenant=%+v err=%v", tenant, err)
	}
	audits, err := repo.ListAudit(context.Background(), "tenant-secret", 10)
	if err != nil || len(audits) != 2 {
		t.Fatalf("credential audit count=%d err=%v", len(audits), err)
	}
	encoded, _ := json.Marshal(audits)
	if strings.Contains(string(encoded), oldKey) || strings.Contains(string(encoded), response.APIKey) || strings.Contains(string(encoded), ciphertext) {
		t.Fatal("credential audit leaked secret material")
	}
}

func TestLegacyTenantCredentialRequiresRotationAndTenantListDoesNotLeakSecrets(t *testing.T) {
	repo := store.NewMemory()
	cipher, _ := security.NewCipher(bytes.Repeat([]byte{6}, 32))
	repo.CreateTenant(context.Background(), domain.Tenant{ID: "tenant-legacy", Name: "Alpakyros LITE", APIKeyHash: security.HashSecret("legacy-key"), Active: true, CreatedAt: time.Now().UTC()})
	h := Server{Repo: repo, Cipher: cipher, AdminTokens: map[string]string{"admin": "super_admin"}}.Handler()

	listed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenants", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(listed, req)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "legacy-key") || strings.Contains(listed.Body.String(), security.HashSecret("legacy-key")) || !strings.Contains(listed.Body.String(), `"api_key_recoverable":false`) {
		t.Fatalf("list code=%d body=%s", listed.Code, listed.Body.String())
	}

	reveal := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/tenants/tenant-legacy/credentials", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(reveal, req)
	if reveal.Code != http.StatusConflict || !strings.Contains(reveal.Body.String(), "API key rotation required") || !strings.Contains(reveal.Body.String(), `"code":"api_key_rotation_required"`) {
		t.Fatalf("legacy reveal code=%d body=%s", reveal.Code, reveal.Body.String())
	}
}

func TestUpdateTenantKeepsAPIKeyAndChangesConnectivity(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-b", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", Name: "Old", APIKeyHash: security.HashSecret("tenant-key"), APIKeyCiphertext: "encrypted-key", Active: true, CreatedAt: time.Now().UTC()})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "super_admin"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/tenants/tenant-a", strings.NewReader(`{"name":"Updated tenant","merchant_id":"merchant-b","site_url":"https://new.example","callback_url":"","webhook_url":"https://new.example/qris","sandbox_mode":true,"use_unique_amount_code":true,"active":false}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"merchant_id":"merchant-b"`) || !strings.Contains(w.Body.String(), `"active":false`) {
		t.Fatalf("update code=%d body=%s", w.Code, w.Body.String())
	}
	stored, err := repo.Tenant(ctx, "tenant-a")
	if err != nil || stored.Name != "Updated tenant" || stored.SiteURL != "https://new.example" || !stored.SandboxMode || !stored.UseUniqueAmountCode || stored.APIKeyHash != security.HashSecret("tenant-key") || stored.APIKeyCiphertext != "encrypted-key" {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	legacy := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/admin/tenants/tenant-a", strings.NewReader(`{"name":"Legacy client update","merchant_id":"merchant-b","site_url":"https://new.example","callback_url":"","webhook_url":"https://new.example/qris","sandbox_mode":true,"active":false}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(legacy, req)
	stored, err = repo.Tenant(ctx, "tenant-a")
	if legacy.Code != http.StatusOK || err != nil || !stored.UseUniqueAmountCode {
		t.Fatalf("legacy update code=%d stored=%+v err=%v", legacy.Code, stored, err)
	}
}

func TestDeleteTenantArchivesAccessAndRetainsHistory(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-delete", Name: "Delete me", APIKeyHash: security.HashSecret("delete-key"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-shared", AccessScope: "all_tenants"})
	repo.CreateTestPayment(ctx, domain.TestPayment{ID: "payment-history", QRISTemplateID: "template-shared", TenantID: "tenant-delete", RequestSource: "tenant_api", Status: domain.InvoiceExpired})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "super_admin"}}.Handler()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/admin/tenants/tenant-delete", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete code=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := repo.Tenant(ctx, "tenant-delete"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted tenant still exists: %v", err)
	}
	payment, err := repo.TestPayment(ctx, "payment-history")
	if err != nil || payment.TenantID != "tenant-delete" {
		t.Fatalf("payment history was not retained: payment=%+v err=%v", payment, err)
	}
	if _, err := repo.TenantByAPIKey(ctx, security.HashSecret("delete-key")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted tenant API key still authenticates: %v", err)
	}

	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-owned", Name: "Has resources", Active: true})
	repo.CreateMerchantAccount(ctx, domain.MerchantAccount{ID: "account-owned", TenantID: "tenant-owned"})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/admin/tenants/tenant-owned", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("owned-resource delete code=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := repo.MerchantAccount(ctx, "tenant-owned", "account-owned"); err != nil {
		t.Fatalf("tenant history resource was removed: %v", err)
	}
}

func TestTenantRefreshAndTransactionHistoryAreIsolated(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant_a", MerchantID: "merchant_a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant_b", MerchantID: "merchant_a", APIKeyHash: security.HashSecret("key-b"), Active: true})
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant_a", Status: domain.ConnectionConnected, UpdatedAt: time.Now().UTC()})
	repo.CreatePortalTransaction(ctx, domain.PortalTransaction{ID: "tx-a", MerchantID: "merchant_a", TenantID: "tenant_a", Reference: "A", PaidAt: time.Now().UTC()})
	repo.CreatePortalTransaction(ctx, domain.PortalTransaction{ID: "tx-b", MerchantID: "merchant_a", TenantID: "tenant_b", Reference: "B", PaidAt: time.Now().UTC()})
	h := Server{Repo: repo}.Handler()

	refresh := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant_a/transactions/refresh", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(refresh, req)
	if refresh.Code != http.StatusAccepted || !strings.Contains(refresh.Body.String(), `"status":"queued"`) {
		t.Fatalf("refresh code=%d body=%s", refresh.Code, refresh.Body.String())
	}
	connection, _ := repo.MerchantConnection(ctx, "merchant_a")
	if connection.LastError != "Tenant requested transaction refresh" || connection.UpdatedAt.IsZero() {
		t.Fatalf("connection=%+v", connection)
	}
	job, claimed, err := repo.ClaimBrowserJob(ctx, "worker", time.Now().UTC(), time.Minute)
	if err != nil || !claimed || job.Kind != "merchant_sync" || job.MerchantID != "merchant_a" {
		t.Fatalf("job=%+v claimed=%v err=%v", job, claimed, err)
	}

	history := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant_a/transactions", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(history, req)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"reference":"A"`) || strings.Contains(history.Body.String(), `"reference":"B"`) {
		t.Fatalf("history code=%d body=%s", history.Code, history.Body.String())
	}

	crossTenant := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant_b/transactions/refresh", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(crossTenant, req)
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant code=%d body=%s", crossTenant.Code, crossTenant.Body.String())
	}
}

func TestAdminHealthIncludesCoreAndMerchantBrowserConnection(t *testing.T) {
	repo := store.NewMemory()
	now := time.Now().UTC().Truncate(time.Second)
	repo.CreateMerchantID(context.Background(), domain.MerchantID{ID: "merchant_1", Name: "Merchant browser", Active: true})
	repo.UpsertMerchantConnection(context.Background(), domain.MerchantConnection{MerchantID: "merchant_1", Status: domain.ConnectionConnected, LastSyncedAt: &now, UpdatedAt: now})
	h := Server{Repo: repo, AdminTokens: map[string]string{"view": "viewer"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/health", nil)
	req.Header.Set("Authorization", "Bearer view")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health code=%d body=%s", w.Code, w.Body.String())
	}
	for _, expected := range []string{`"id":"backend-api"`, `"kind":"database"`, `"id":"browser-merchant_1"`, `"kind":"browser_session"`, `"status":"operational"`, `"last_synced_at":"`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("health response missing %s: %s", expected, w.Body.String())
		}
	}
}

func TestAdminListLimit(t *testing.T) {
	r := store.NewMemory()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		id := string(rune('a' + i))
		r.CreateInvoice(ctx, domain.Invoice{ID: id, TenantID: "t", IdempotencyKey: id, CreatedAt: time.Unix(int64(i), 0)})
		r.AppendAudit(ctx, domain.AuditEvent{ID: id, TenantID: "t", CreatedAt: time.Unix(int64(i), 0)})
	}
	h := Server{Repo: r, AdminTokens: map[string]string{"view": "viewer"}}.Handler()
	for _, path := range []string{"/admin/invoices?tenant_id=t&limit=2", "/admin/audit-events?tenant_id=t&limit=2"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer view")
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK || strings.Count(w.Body.String(), `"id"`) != 2 {
			t.Fatalf("path=%s code=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/invoices?limit=501", nil)
	req.Header.Set("Authorization", "Bearer view")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminQRISTestWorkflow(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	png, err := qrcode.Encode(staticPayload, qrcode.Medium, 320)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err = form.WriteField("name", "Store QRIS"); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("image", "qris.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	repo := store.NewMemory()
	repo.CreateMerchantID(context.Background(), domain.MerchantID{ID: "merchant-test", InteractiveMerchantID: "interactive-test", Name: "Test merchant", Active: true})
	repo.UpsertMerchantConnection(context.Background(), domain.MerchantConnection{MerchantID: "merchant-test", Status: domain.ConnectionConnected, UpdatedAt: time.Now().UTC()})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "super_admin"}}.Handler()
	upload := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/qris-templates", &body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", form.FormDataContentType())
	h.ServeHTTP(upload, req)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload code=%d body=%s", upload.Code, upload.Body.String())
	}
	var template domain.QRISTemplate
	if err = json.Unmarshal(upload.Body.Bytes(), &template); err != nil || template.ID == "" {
		t.Fatalf("template=%+v err=%v", template, err)
	}
	create := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/qris-test-payments", strings.NewReader(`{"qris_template_id":"`+template.ID+`","amount":50000}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(create, req)
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), `"status":"pending"`) || !strings.Contains(create.Body.String(), `"merchant_id":"merchant-test"`) || !strings.Contains(create.Body.String(), `"match_confidence":"waiting_first_check"`) || !strings.Contains(create.Body.String(), "540550000") || !strings.Contains(create.Body.String(), `"next_check_at"`) {
		t.Fatalf("payment code=%d body=%s", create.Code, create.Body.String())
	}
	connection, _ := repo.MerchantConnection(context.Background(), "merchant-test")
	if connection.LastError != "" || connection.UpdatedAt.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("connection=%+v", connection)
	}
}

func TestGlobalTransactionsIncludeExpiredQRISTestCheck(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	now := time.Now().UTC()
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-global"})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID: "test-global", QRISTemplateID: "template-global", MerchantID: "merchant-global",
		Amount: 12500, Status: domain.InvoicePending, RequestSource: "admin_qris_test",
		MatchConfidence: "browser_sync_queued", CreatedAt: now.Add(-31 * time.Minute),
		UpdatedAt: now.Add(-31 * time.Minute), ExpiresAt: now.Add(-time.Minute),
	})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "viewer"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/global-transactions?limit=10", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"event_type":"qris_test_check"`) || !strings.Contains(w.Body.String(), `"status":"expired"`) || !strings.Contains(w.Body.String(), `"validation":"expired_no_match"`) || !strings.Contains(w.Body.String(), `"expires_at"`) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTenantTransactionLedgerIncludesProductionInvoicesAndSandboxQRIS(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	now := time.Now().UTC()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-production", Name: "Production tenant", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-sandbox", Name: "Sandbox tenant", SandboxMode: true, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-ledger"})
	_, _, _ = repo.CreateInvoice(ctx, domain.Invoice{
		ID: "invoice-production", TenantID: "tenant-production", IdempotencyKey: "order-production",
		Amount: 25000, Currency: "IDR", Status: domain.InvoicePaid,
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now, ExpiresAt: now.Add(29 * time.Minute),
	})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID: "qris-sandbox", IdempotencyKey: "order-sandbox", QRISTemplateID: "template-ledger",
		MerchantID: "merchant-ledger", TenantID: "tenant-sandbox", Amount: 31000,
		Status: domain.InvoicePending, RequestSource: "tenant_api", MatchConfidence: "waiting_first_check", SandboxMode: true,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID: "admin-test", QRISTemplateID: "template-ledger", Amount: 1000,
		Status: domain.InvoicePending, RequestSource: "admin_qris_test",
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	})
	for i := 0; i < 500; i++ {
		repo.CreateTestPayment(ctx, domain.TestPayment{
			ID: fmt.Sprintf("newer-admin-test-%03d", i), QRISTemplateID: "template-ledger", Amount: 1000,
			Status: domain.InvoicePending, RequestSource: "admin_qris_test",
			CreatedAt: now.Add(time.Duration(i+1) * time.Second), UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
		})
	}

	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "viewer"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/tenant-transactions?limit=10", nil)
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"invoice-production"`) || !strings.Contains(w.Body.String(), `"mode":"production"`) {
		t.Fatalf("production ledger code=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"id":"qris-sandbox"`) || !strings.Contains(w.Body.String(), `"mode":"sandbox"`) || !strings.Contains(w.Body.String(), `"kind":"qris"`) {
		t.Fatalf("sandbox ledger code=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "admin-test") {
		t.Fatalf("admin QRIS test leaked into tenant ledger body=%s", w.Body.String())
	}
}

func TestTenantCanGenerateDynamicQRISOnlyFromOwnedEnabledTemplate(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-a"), SandboxMode: true, Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-b", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-b"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-a", TenantID: "tenant-a", AccessScope: "selected_tenant", Name: "Store", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 1, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", Name: "Shared", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 60, Active: true})
	h := Server{Repo: repo}.Handler()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/qris/dynamic", strings.NewReader(`{"template_id":"template-a","amount":50000}`))
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"qr_payload":"000201010212`) || !strings.Contains(w.Body.String(), `"qr_png_base64":"`) || !strings.Contains(w.Body.String(), `"amount":50000`) || !strings.Contains(w.Body.String(), `"sandbox_mode":true`) {
		t.Fatalf("generate code=%d body=%s", w.Code, w.Body.String())
	}

	isolated := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-b/qris/dynamic", strings.NewReader(`{"template_id":"template-a","amount":50000}`))
	req.Header.Set("X-API-Key", "key-b")
	h.ServeHTTP(isolated, req)
	if isolated.Code != http.StatusNotFound {
		t.Fatalf("cross tenant code=%d body=%s", isolated.Code, isolated.Body.String())
	}

	limited := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/qris/dynamic", strings.NewReader(`{"template_id":"template-a","amount":50001}`))
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(limited, req)
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit code=%d retry=%s body=%s", limited.Code, limited.Header().Get("Retry-After"), limited.Body.String())
	}

	shared := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-b/qris/dynamic", strings.NewReader(`{"template_id":"template-all","amount":25000}`))
	req.Header.Set("X-API-Key", "key-b")
	h.ServeHTTP(shared, req)
	if shared.Code != http.StatusCreated {
		t.Fatalf("shared template code=%d body=%s", shared.Code, shared.Body.String())
	}
}

func TestTenantCanDiscoverOnlyUsableQRSTemplates(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-owned", TenantID: "tenant-a", AccessScope: "selected_tenant", Name: "QRIS Tenant A", StaticPayload: "secret-payload", ImageData: []byte("secret-image"), StaticToDynamic: true, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-other", TenantID: "tenant-b", AccessScope: "selected_tenant", Name: "QRIS Tenant B", StaticToDynamic: true, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-shared", AccessScope: "all_tenants", Name: "QRIS Bersama", StaticToDynamic: true, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-inactive", AccessScope: "all_tenants", Name: "Nonaktif", StaticToDynamic: true, Active: false})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-static", AccessScope: "all_tenants", Name: "Statis saja", StaticToDynamic: false, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-invalid-scope", AccessScope: "unexpected", Name: "Scope rusak", StaticToDynamic: true, Active: true})

	h := Server{Repo: repo}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-a/qris/templates", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"template-owned"`) || !strings.Contains(w.Body.String(), `"id":"template-shared"`) {
		t.Fatalf("discover code=%d body=%s", w.Code, w.Body.String())
	}
	for _, forbidden := range []string{"template-other", "template-inactive", "template-static", "template-invalid-scope", "static_payload", "image_data", "secret-payload", "c2VjcmV0LWltYWdl"} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("discovery leaked %q body=%s", forbidden, w.Body.String())
		}
	}
}

func TestTenantQRISCreateExplainsMissingOrUnknownTemplateID(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-inactive", Active: false, StaticToDynamic: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-static", Active: true, StaticToDynamic: false})
	h := Server{Repo: repo}.Handler()

	for name, test := range map[string]struct {
		body string
		code int
	}{
		"missing":  {body: `{"amount":50000}`, code: http.StatusBadRequest},
		"unknown":  {body: `{"template_id":"not-registered","amount":50000}`, code: http.StatusNotFound},
		"inactive": {body: `{"template_id":"template-inactive","amount":50000}`, code: http.StatusConflict},
		"static":   {body: `{"template_id":"template-static","amount":50000}`, code: http.StatusConflict},
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(test.body))
			req.Header.Set("X-API-Key", "key-a")
			h.ServeHTTP(w, req)
			if w.Code != test.code || !strings.Contains(w.Body.String(), "/v1/tenants/tenant-a/qris/templates") {
				t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

type qrisTemplateErrorRepository struct {
	store.Repository
}

func (qrisTemplateErrorRepository) QRISTemplate(context.Context, string) (domain.QRISTemplate, error) {
	return domain.QRISTemplate{}, errors.New("database unavailable")
}

func TestTenantQRISCreatePreservesTemplateRepositoryFailure(t *testing.T) {
	base := store.NewMemory()
	base.CreateTenant(context.Background(), domain.Tenant{ID: "tenant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	h := Server{Repo: qrisTemplateErrorRepository{Repository: base}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(`{"template_id":"template-a","amount":50000}`))
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTenantQRISTransactionRoutesPersistAndAreIdempotent(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-b", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-b"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 1, Active: true})
	h := Server{Repo: repo}.Handler()

	create := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"template_id":"template-all","amount":1013,"idempotency_key":"order-1013","expires_in_seconds":60}`))
		req.Header.Set("X-API-Key", "key-a")
		h.ServeHTTP(w, req)
		return w
	}
	canonical := create("/v1/tenants/tenant-a/transactions/qris")
	if canonical.Code != http.StatusCreated {
		t.Fatalf("canonical code=%d body=%s", canonical.Code, canonical.Body.String())
	}
	alias := create("/v1/tenants/tenant-a/qris/dynamic")
	if alias.Code != http.StatusOK {
		t.Fatalf("alias replay code=%d body=%s", alias.Code, alias.Body.String())
	}
	var first, replay domain.TestPayment
	if err := json.Unmarshal(canonical.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(alias.Body.Bytes(), &replay); err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || replay.ID != first.ID || first.RequestSource != "tenant_api" || first.TenantID != "tenant-a" || first.MerchantID != "merchant-a" || first.UniqueCode == "" {
		t.Fatalf("first=%+v replay=%+v", first, replay)
	}
	if !strings.Contains(alias.Body.String(), `"template_id":"template-all"`) || !strings.Contains(alias.Body.String(), `"currency":"IDR"`) {
		t.Fatalf("legacy response fields missing: %s", alias.Body.String())
	}

	detail := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-a/transactions/qris/"+first.ID, nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(detail, req)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"status":"pending"`) || !strings.Contains(detail.Body.String(), `"poll_after_seconds":`) || detail.Header().Get("Retry-After") == "" {
		t.Fatalf("detail code=%d body=%s", detail.Code, detail.Body.String())
	}

	qr := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-a/transactions/qris/"+first.ID+"/qr", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(qr, req)
	if qr.Code != http.StatusOK || qr.Header().Get("Content-Type") != "image/png" || qr.Body.Len() < 100 {
		t.Fatalf("qr code=%d content-type=%q bytes=%d", qr.Code, qr.Header().Get("Content-Type"), qr.Body.Len())
	}

	history := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-a/transactions", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(history, req)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), first.ID) || !strings.Contains(history.Body.String(), `"request_source":"tenant_api"`) {
		t.Fatalf("history code=%d body=%s", history.Code, history.Body.String())
	}

	crossTenant := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-b/transactions/qris/"+first.ID, nil)
	req.Header.Set("X-API-Key", "key-b")
	h.ServeHTTP(crossTenant, req)
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross tenant code=%d body=%s", crossTenant.Code, crossTenant.Body.String())
	}
	if job, claimed, err := repo.ClaimBrowserJob(ctx, "status-probe", time.Now().UTC(), time.Minute); err != nil || claimed {
		t.Fatalf("status GET created browser job claimed=%v job=%+v err=%v", claimed, job, err)
	}
}

func TestTenantQRISCancelLifecycle(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	now := time.Now().UTC()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-b", APIKeyHash: security.HashSecret("key-b"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-a"})
	next := now.Add(-time.Second)
	pending := domain.TestPayment{ID: "payment-pending", QRISTemplateID: "template-a", MerchantID: "merchant-a", TenantID: "tenant-a", Amount: 1000, PayableAmount: 1000, DynamicPayload: "payload", Status: domain.InvoicePending, RequestSource: "tenant_api", MatchConfidence: "waiting_first_check", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute), NextCheckAt: &next}
	if err := repo.CreateTestPayment(ctx, pending); err != nil {
		t.Fatal(err)
	}
	for _, terminal := range []domain.InvoiceStatus{domain.InvoicePaid, domain.InvoiceExpired, domain.InvoiceFailed} {
		payment := pending
		payment.ID, payment.Status, payment.NextCheckAt = "payment-"+string(terminal), terminal, nil
		if err := repo.CreateTestPayment(ctx, payment); err != nil {
			t.Fatal(err)
		}
	}
	pastDeadline := pending
	pastDeadline.ID = "payment-past-deadline"
	pastDeadline.Amount = 2000
	pastDeadline.PayableAmount = 2000
	pastDeadline.ExpiresAt = now.Add(-time.Second)
	if err := repo.CreateTestPayment(ctx, pastDeadline); err != nil {
		t.Fatal(err)
	}
	h := Server{Repo: repo}.Handler()
	cancel := func(key, id string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris/"+id+"/cancel", nil)
		req.Header.Set("X-API-Key", key)
		h.ServeHTTP(w, req)
		return w
	}

	first := cancel("key-a", pending.ID)
	if first.Code != http.StatusOK || first.Header().Get("Retry-After") != "" || !strings.Contains(first.Body.String(), `"status":"cancelled"`) || !strings.Contains(first.Body.String(), `"match_confidence":"cancelled_by_tenant"`) || !strings.Contains(first.Body.String(), `"next_check_at":null`) {
		t.Fatalf("first cancel code=%d body=%s", first.Code, first.Body.String())
	}
	queued, err := repo.PendingTestPayments(ctx, now, 10)
	if err != nil {
		t.Fatalf("cancelled payment still queued: %+v err=%v", queued, err)
	}
	for _, payment := range queued {
		if payment.ID == pending.ID {
			t.Fatalf("cancelled payment still queued: %+v", queued)
		}
	}
	audits, err := repo.ListAudit(ctx, "tenant-a", 20)
	if err != nil {
		t.Fatal(err)
	}
	cancelAudits := 0
	for _, event := range audits {
		if event.Action == "qris.transaction_cancelled" && event.ResourceID == pending.ID {
			cancelAudits++
		}
	}
	if cancelAudits != 1 {
		t.Fatalf("cancel audit count=%d events=%+v", cancelAudits, audits)
	}

	replay := cancel("key-a", pending.ID)
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("replay code=%d body=%s", replay.Code, replay.Body.String())
	}
	audits, _ = repo.ListAudit(ctx, "tenant-a", 20)
	cancelAudits = 0
	for _, event := range audits {
		if event.Action == "qris.transaction_cancelled" && event.ResourceID == pending.ID {
			cancelAudits++
		}
	}
	if cancelAudits != 1 {
		t.Fatalf("idempotent cancel duplicated audit count=%d", cancelAudits)
	}

	for _, terminal := range []domain.InvoiceStatus{domain.InvoicePaid, domain.InvoiceExpired, domain.InvoiceFailed} {
		response := cancel("key-a", "payment-"+string(terminal))
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"status":"`+string(terminal)+`"`) {
			t.Fatalf("terminal=%s code=%d body=%s", terminal, response.Code, response.Body.String())
		}
	}
	pastDeadlineResponse := cancel("key-a", pastDeadline.ID)
	if pastDeadlineResponse.Code != http.StatusConflict || !strings.Contains(pastDeadlineResponse.Body.String(), `"status":"expired"`) {
		t.Fatalf("past deadline code=%d body=%s", pastDeadlineResponse.Code, pastDeadlineResponse.Body.String())
	}

	crossTenant := cancel("key-b", pending.ID)
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross tenant code=%d body=%s", crossTenant.Code, crossTenant.Body.String())
	}
	missing := cancel("key-a", "missing-payment")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing transaction code=%d body=%s", missing.Code, missing.Body.String())
	}

	qr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-a/transactions/qris/"+pending.ID+"/qr", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(qr, req)
	if qr.Code != http.StatusGone {
		t.Fatalf("cancelled qr code=%d body=%s", qr.Code, qr.Body.String())
	}
}

func TestTenantQRISUniqueAmountAllocatesDistinctPayableAmounts(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", UseUniqueAmountCode: true, UniqueAmountCooldownMinutes: 45, APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 10, Active: true})
	h := Server{Repo: repo, UniqueAmountCodeOrder: func() ([]int64, error) {
		return []int64{37, 12, 88}, nil
	}}.Handler()

	for i, key := range []string{"order-a", "order-b"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(`{"template_id":"template-all","amount":10000,"idempotency_key":"`+key+`"}`))
		req.Header.Set("X-API-Key", "key-a")
		h.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("request %d code=%d body=%s", i, w.Code, w.Body.String())
		}
		var payment domain.TestPayment
		if err := json.Unmarshal(w.Body.Bytes(), &payment); err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		wantCodes := []int64{37, 12}
		wantPayable := int64(10000) + wantCodes[i]
		if payment.Amount != 10000 || payment.PayableAmount != wantPayable || payment.UniqueAmountCode != wantCodes[i] {
			t.Fatalf("request %d payment=%+v", i, payment)
		}
		if response["requested_amount"] != float64(10000) || response["payable_amount"] != float64(wantPayable) || response["unique_amount_code"] != float64(wantCodes[i]) {
			t.Fatalf("request %d response=%s", i, w.Body.String())
		}
	}
}

func TestSecureUniqueAmountCodeOrderIsPermutation(t *testing.T) {
	codes, err := secureUniqueAmountCodeOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 99 {
		t.Fatalf("code count=%d", len(codes))
	}
	seen := make(map[int64]bool, len(codes))
	for _, code := range codes {
		if code < 1 || code > 99 || seen[code] {
			t.Fatalf("invalid permutation code=%d seen=%v", code, seen[code])
		}
		seen[code] = true
	}
}

func TestTenantCooldownMinutesMustBeBetweenThirtyAndSixty(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", Name: "Tenant", UniqueAmountCooldownMinutes: 30, Active: true})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "super_admin"}}.Handler()

	for _, minutes := range []int{29, 61} {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"name":"Tenant","merchant_id":"merchant-a","use_unique_amount_code":true,"unique_amount_cooldown_minutes":%d,"active":true}`, minutes)
		req := httptest.NewRequest(http.MethodPut, "/admin/tenants/tenant-a", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer admin")
		h.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("minutes=%d code=%d body=%s", minutes, w.Code, w.Body.String())
		}
	}
}

func TestTenantQRISWithoutUniqueAmountRejectsOverlappingNominal(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 10, Active: true})
	h := Server{Repo: repo}.Handler()

	for i, key := range []string{"order-a", "order-b"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(`{"template_id":"template-all","amount":10000,"idempotency_key":"`+key+`"}`))
		req.Header.Set("X-API-Key", "key-a")
		h.ServeHTTP(w, req)
		want := http.StatusCreated
		if i == 1 {
			want = http.StatusConflict
		}
		if w.Code != want {
			t.Fatalf("request %d code=%d body=%s", i, w.Code, w.Body.String())
		}
	}
}

func TestAdminQRISTestCannotReuseTenantReservedPayableAmount(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", UseUniqueAmountCode: true, APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 10, Active: true})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "operator"}, UniqueAmountCodeOrder: func() ([]int64, error) { return []int64{1}, nil }}.Handler()

	tenantRequest := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(`{"template_id":"template-all","amount":10000,"idempotency_key":"order-a"}`))
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(tenantRequest, req)
	if tenantRequest.Code != http.StatusCreated {
		t.Fatalf("tenant code=%d body=%s", tenantRequest.Code, tenantRequest.Body.String())
	}

	adminRequest := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/qris-test-payments", strings.NewReader(`{"qris_template_id":"template-all","amount":10001}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(adminRequest, req)
	if adminRequest.Code != http.StatusConflict {
		t.Fatalf("admin code=%d body=%s", adminRequest.Code, adminRequest.Body.String())
	}
}

func TestTenantQRISTransactionIdempotencyRejectsDifferentExpiry(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 10, Active: true})
	h := Server{Repo: repo}.Handler()
	for i, expiry := range []int{60, 120} {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"template_id":"template-all","amount":1013,"idempotency_key":"same-order","expires_in_seconds":%d}`, expiry)
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(body))
		req.Header.Set("X-API-Key", "key-a")
		h.ServeHTTP(w, req)
		want := http.StatusCreated
		if i == 1 {
			want = http.StatusConflict
		}
		if w.Code != want {
			t.Fatalf("expiry=%d code=%d body=%s", expiry, w.Code, w.Body.String())
		}
	}
}

func TestExpiredTenantQRISTransactionDoesNotExposePayableQR(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	now := time.Now().UTC()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", StaticPayload: staticPayload})
	payload, _ := qrisservice.Convert(staticPayload, 1013)
	repo.CreateTestPayment(ctx, domain.TestPayment{ID: "expired-payment", QRISTemplateID: "template-all", MerchantID: "merchant-a", TenantID: "tenant-a", Amount: 1013, DynamicPayload: payload, Status: domain.InvoicePending, RequestSource: "tenant_api", CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(-time.Minute)})
	h := Server{Repo: repo}.Handler()

	detail := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-a/transactions/qris/expired-payment", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(detail, req)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"status":"expired"`) || strings.Contains(detail.Body.String(), `"qr_png_base64"`) || strings.Contains(detail.Body.String(), `"qr_payload"`) {
		t.Fatalf("detail code=%d body=%s", detail.Code, detail.Body.String())
	}

	qr := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant-a/transactions/qris/expired-payment/qr", nil)
	req.Header.Set("X-API-Key", "key-a")
	h.ServeHTTP(qr, req)
	if qr.Code != http.StatusGone {
		t.Fatalf("qr code=%d body=%s", qr.Code, qr.Body.String())
	}
}

func TestTenantQRISTransactionConcurrentIdempotencyCreatesOneLedgerEntry(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 1, Active: true})
	h := Server{Repo: repo}.Handler()

	const requests = 12
	ids := make(chan string, requests)
	errs := make(chan string, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(`{"template_id":"template-all","amount":1013,"idempotency_key":"same-order"}`))
			req.Header.Set("X-API-Key", "key-a")
			h.ServeHTTP(w, req)
			if w.Code != http.StatusCreated && w.Code != http.StatusOK {
				errs <- fmt.Sprintf("code=%d body=%s", w.Code, w.Body.String())
				return
			}
			var payment domain.TestPayment
			if err := json.Unmarshal(w.Body.Bytes(), &payment); err != nil {
				errs <- err.Error()
				return
			}
			ids <- payment.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var want string
	for id := range ids {
		if want == "" {
			want = id
		}
		if id != want {
			t.Fatalf("idempotent requests returned %q and %q", want, id)
		}
	}
	payments, _ := repo.ListTenantTestPayments(ctx, "tenant-a", 100)
	if len(payments) != 1 {
		t.Fatalf("ledger entries=%d payments=%+v", len(payments), payments)
	}
}

func TestTenantQRISTransactionRateLimitCountsDistinctIdempotencyKeys(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-a", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", APIKeyHash: security.HashSecret("key-a"), Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-all", AccessScope: "all_tenants", StaticPayload: staticPayload, StaticToDynamic: true, MaxRequestsPM: 1, Active: true})
	h := Server{Repo: repo}.Handler()

	statuses := []int{}
	for i, key := range []string{"order-a", "order-b"} {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"template_id":"template-all","amount":%d,"idempotency_key":"%s"}`, 1013+i, key)
		req := httptest.NewRequest(http.MethodPost, "/v1/tenants/tenant-a/transactions/qris", strings.NewReader(body))
		req.Header.Set("X-API-Key", "key-a")
		h.ServeHTTP(w, req)
		statuses = append(statuses, w.Code)
	}
	if statuses[0] != http.StatusCreated || statuses[1] != http.StatusTooManyRequests {
		t.Fatalf("statuses=%v", statuses)
	}
	payments, _ := repo.ListTenantTestPayments(ctx, "tenant-a", 100)
	if len(payments) != 1 {
		t.Fatalf("ledger entries=%d", len(payments))
	}
}

func TestAdminCanEditQRISTemplateAccessAndRateLimit(t *testing.T) {
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-a", Name: "Old", AccessScope: "all_tenants", MaxRequestsPM: 60, Active: true})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "operator"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/qris-templates/template-a", strings.NewReader(`{"name":"Tenant QRIS","tenant_id":"tenant-a","access_scope":"selected_tenant","static_to_dynamic":true,"max_requests_per_minute":12,"active":true}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"max_requests_per_minute":12`) || !strings.Contains(w.Body.String(), `"tenant_id":"tenant-a"`) {
		t.Fatalf("update code=%d body=%s", w.Code, w.Body.String())
	}
	stored, _ := repo.QRISTemplate(ctx, "template-a")
	if stored.AccessScope != "selected_tenant" || !stored.StaticToDynamic || stored.MaxRequestsPM != 12 {
		t.Fatalf("stored=%+v", stored)
	}
}

func TestCreateTestPaymentInjectsUniqueCode(t *testing.T) {
	const staticPayload = "00020101021126570011ID.DANA.WWW011893600915303088327702090308832770303UMI51440014ID.CO.QRIS.WWW0215ID10265298200310303UMI5204504553033605802ID5906ByAsta6011Kab. Malang61056516463049095"
	repo := store.NewMemory()
	ctx := context.Background()
	repo.CreateMerchantID(ctx, domain.MerchantID{ID: "merchant-test", Name: "Test merchant", Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test", StaticPayload: staticPayload, Active: true})
	h := Server{Repo: repo, AdminTokens: map[string]string{"admin": "operator"}}.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/qris-test-payments", strings.NewReader(`{"qris_template_id":"template-test","amount":50000}`))
	req.Header.Set("Authorization", "Bearer admin")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var payment domain.TestPayment
	if err := json.Unmarshal(w.Body.Bytes(), &payment); err != nil {
		t.Fatal(err)
	}
	if len(payment.UniqueCode) != 8 {
		t.Fatalf("unique_code=%q", payment.UniqueCode)
	}
	if !strings.Contains(payment.DynamicPayload, payment.UniqueCode) {
		t.Fatalf("dynamic payload does not embed unique code: %s", payment.DynamicPayload)
	}
	stored, _ := repo.TestPayment(ctx, payment.ID)
	if stored.UniqueCode != payment.UniqueCode || !strings.Contains(stored.DynamicPayload, payment.UniqueCode) {
		t.Fatalf("stored=%+v", stored)
	}
}

func TestPaymentSessionHTTPCreateSnapshotCancelAndPublicIsolation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	repo := store.NewMemory()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", APIKeyHash: security.HashSecret("key-a"), APIKeyCiphertext: "api-secret", WebhookSecretCiphertext: "webhook-secret", Active: true})
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-b", APIKeyHash: security.HashSecret("key-b"), Active: true})
	repo.CreateInvoice(ctx, domain.Invoice{ID: "invoice-a", TenantID: "tenant-a", IdempotencyKey: "invoice-a", Amount: 100037, Currency: "IDR", Description: "public order", QRPayload: "qris-payload", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)})
	repo.CreateInvoice(ctx, domain.Invoice{ID: "redirect-invoice", TenantID: "tenant-a", IdempotencyKey: "redirect", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)})
	service := gateway.PaymentSessionService{Repo: repo, Now: func() time.Time { return now }}
	h := Server{Repo: repo, PaymentSessions: service, PublicPaymentBaseURL: "https://pay.example.test"}.Handler()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/payment-sessions", strings.NewReader(`{"invoice_id":"invoice-a"}`))
	createReq.Header.Set("X-API-Key", "key-a")
	createRes := httptest.NewRecorder()
	h.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create code=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var created PublicPaymentSessionResponse
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != "payment_pending" || created.CheckoutURL == "" || !strings.HasPrefix(created.CheckoutURL, "https://pay.example.test/pay/") {
		t.Fatalf("unexpected create response: %+v", created)
	}
	if strings.Contains(createRes.Body.String(), "api-secret") || strings.Contains(createRes.Body.String(), "webhook-secret") || strings.Contains(createRes.Body.String(), "key-a") {
		t.Fatalf("public response leaked secret material: %s", createRes.Body.String())
	}
	token := strings.TrimPrefix(created.CheckoutURL, "https://pay.example.test/pay/")

	getReq := httptest.NewRequest(http.MethodGet, "/v1/payment-sessions/"+token, nil)
	getReq.Header.Set("Origin", "http://127.0.0.1:3000")
	getRes := httptest.NewRecorder()
	h.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("snapshot code=%d body=%s", getRes.Code, getRes.Body.String())
	}
	if got := getRes.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:3000" {
		t.Fatalf("payment session CORS origin=%q", got)
	}
	var snapshot PublicPaymentSessionResponse
	if err := json.Unmarshal(getRes.Body.Bytes(), &snapshot); err != nil || snapshot.Status != "payment_pending" || snapshot.Amount != 100037 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	wrongTokenRes := httptest.NewRecorder()
	h.ServeHTTP(wrongTokenRes, httptest.NewRequest(http.MethodGet, "/v1/payment-sessions/not-a-real-token", nil))
	if wrongTokenRes.Code != http.StatusNotFound {
		t.Fatalf("wrong token should be 404, got %d", wrongTokenRes.Code)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/v1/payment-sessions/"+token+"/cancel", nil)
	cancelRes := httptest.NewRecorder()
	h.ServeHTTP(cancelRes, cancelReq)
	if cancelRes.Code != http.StatusOK {
		t.Fatalf("cancel code=%d body=%s", cancelRes.Code, cancelRes.Body.String())
	}
	if err := json.Unmarshal(cancelRes.Body.Bytes(), &snapshot); err != nil || snapshot.Status != "cancelled" {
		t.Fatalf("cancel snapshot=%+v err=%v", snapshot, err)
	}
	invoiceAfterCancel, err := repo.Invoice(ctx, "tenant-a", "invoice-a")
	if err != nil || invoiceAfterCancel.Status != domain.InvoicePending {
		t.Fatalf("session cancel changed invoice financial state: %+v err=%v", invoiceAfterCancel, err)
	}
	repeatRes := httptest.NewRecorder()
	h.ServeHTTP(repeatRes, httptest.NewRequest(http.MethodPost, "/v1/payment-sessions/"+token+"/cancel", nil))
	if repeatRes.Code != http.StatusOK {
		t.Fatalf("cancel after cancelled should be idempotent, got %d", repeatRes.Code)
	}
	invalidRedirectReq := httptest.NewRequest(http.MethodPost, "/v1/payment-sessions", strings.NewReader(`{"invoice_id":"redirect-invoice","success_url":"https://attacker.example"}`))
	invalidRedirectReq.Header.Set("X-API-Key", "key-a")
	invalidRedirectRes := httptest.NewRecorder()
	h.ServeHTTP(invalidRedirectRes, invalidRedirectReq)
	if invalidRedirectRes.Code != http.StatusBadRequest || !strings.Contains(invalidRedirectRes.Body.String(), `"invalid_redirect"`) {
		t.Fatalf("invalid redirect code=%d body=%s", invalidRedirectRes.Code, invalidRedirectRes.Body.String())
	}

	wrongTenantReq := httptest.NewRequest(http.MethodPost, "/v1/payment-sessions", strings.NewReader(`{"invoice_id":"invoice-a"}`))
	wrongTenantReq.Header.Set("X-API-Key", "key-b")
	wrongTenantRes := httptest.NewRecorder()
	h.ServeHTTP(wrongTenantRes, wrongTenantReq)
	if wrongTenantRes.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant invoice code=%d body=%s", wrongTenantRes.Code, wrongTenantRes.Body.String())
	}
}

func TestPaymentSessionHTTPResolvesInvoicePaidAndExpiry(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	repo := store.NewMemory()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant", APIKeyHash: security.HashSecret("key"), Active: true})
	repo.CreateInvoice(ctx, domain.Invoice{ID: "paid-invoice", TenantID: "tenant", IdempotencyKey: "paid", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)})
	repo.CreateInvoice(ctx, domain.Invoice{ID: "exp-invoice", TenantID: "tenant", IdempotencyKey: "exp", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})
	clock := now
	h := Server{Repo: repo, PaymentSessions: gateway.PaymentSessionService{Repo: repo, Now: func() time.Time { return clock }}, PublicPaymentBaseURL: "https://pay.example.test"}.Handler()

	create := func(invoice string) string {
		req := httptest.NewRequest(http.MethodPost, "/v1/payment-sessions", strings.NewReader(`{"invoice_id":"`+invoice+`"}`))
		req.Header.Set("X-API-Key", "key")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusCreated {
			t.Fatalf("create %s code=%d body=%s", invoice, res.Code, res.Body.String())
		}
		var v PublicPaymentSessionResponse
		_ = json.Unmarshal(res.Body.Bytes(), &v)
		return strings.TrimPrefix(v.CheckoutURL, "https://pay.example.test/pay/")
	}
	paidToken := create("paid-invoice")
	paid, _ := repo.Invoice(ctx, "tenant", "paid-invoice")
	paid.Status = domain.InvoicePaid
	repo.UpdateInvoice(ctx, paid)
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/v1/payment-sessions/"+paidToken, nil))
	var paidSnapshot PublicPaymentSessionResponse
	_ = json.Unmarshal(get.Body.Bytes(), &paidSnapshot)
	if get.Code != http.StatusOK || paidSnapshot.Status != "paid" {
		t.Fatalf("paid resolution code=%d snapshot=%+v", get.Code, paidSnapshot)
	}
	paidCancel := httptest.NewRecorder()
	h.ServeHTTP(paidCancel, httptest.NewRequest(http.MethodPost, "/v1/payment-sessions/"+paidToken+"/cancel", nil))
	if paidCancel.Code != http.StatusConflict {
		t.Fatalf("paid cancel should conflict, got %d body=%s", paidCancel.Code, paidCancel.Body.String())
	}

	expToken := create("exp-invoice")
	clock = now.Add(11 * time.Minute)
	get = httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/v1/payment-sessions/"+expToken, nil))
	var expiredSnapshot PublicPaymentSessionResponse
	_ = json.Unmarshal(get.Body.Bytes(), &expiredSnapshot)
	if get.Code != http.StatusOK || expiredSnapshot.Status != "expired" {
		t.Fatalf("expiry resolution code=%d snapshot=%+v", get.Code, expiredSnapshot)
	}
	expiredCancel := httptest.NewRecorder()
	h.ServeHTTP(expiredCancel, httptest.NewRequest(http.MethodPost, "/v1/payment-sessions/"+expToken+"/cancel", nil))
	if expiredCancel.Code != http.StatusConflict {
		t.Fatalf("expired cancel should conflict, got %d body=%s", expiredCancel.Code, expiredCancel.Body.String())
	}
}
