package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	qrisprovider "xloyal/backend/internal/provider"
	qrisservice "xloyal/backend/internal/qris"
	"xloyal/backend/internal/security"
	"xloyal/backend/internal/store"
)

type Server struct {
	Repo                  store.Repository
	Gateway               gateway.Service
	Cipher                *security.Cipher
	AdminTokens           map[string]string
	ManualLogin           func(context.Context, domain.MerchantConnection) error
	WebhookSecret         string
	WebhookSignalPath     string
	UniqueAmountCodeOrder func() ([]int64, error)
	PaymentSessions       gateway.PaymentSessionService
	PublicPaymentBaseURL  string
	SSE                   *PaymentSSEHub
}

func (s Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /v1/health", s.health)
	m.HandleFunc("OPTIONS /v1/{rest...}", tenantPreflight)
	m.HandleFunc("POST /internal/github/webhook", s.githubWebhook)
	m.HandleFunc("POST /v1/payment-sessions", s.public(s.createPaymentSession))
	m.HandleFunc("GET /v1/payment-sessions/{token}", s.publicPayment(s.getPaymentSession))
	m.HandleFunc("GET /v1/payment-sessions/{token}/events", s.publicPayment(s.paymentSessionEvents))
	m.HandleFunc("POST /v1/payment-sessions/{token}/cancel", s.publicPayment(s.cancelPaymentSession))
	m.HandleFunc("POST /v1/tenants/{tenant_id}/invoices", s.public(s.createInvoice))
	m.HandleFunc("POST /v1/tenants/{tenant_id}/transactions/refresh", s.public(s.refreshTenantTransactions))
	m.HandleFunc("GET /v1/tenants/{tenant_id}/transactions", s.public(s.listTenantTransactions))
	m.HandleFunc("POST /v1/tenants/{tenant_id}/transactions/qris", s.public(s.createTenantQRISTransaction))
	m.HandleFunc("GET /v1/tenants/{tenant_id}/transactions/qris/{transaction_id}", s.public(s.getTenantQRISTransaction))
	m.HandleFunc("POST /v1/tenants/{tenant_id}/transactions/qris/{transaction_id}/cancel", s.public(s.cancelTenantQRISTransaction))
	m.HandleFunc("GET /v1/tenants/{tenant_id}/transactions/qris/{transaction_id}/qr", s.public(s.getTenantQRISTransactionQR))
	m.HandleFunc("GET /v1/tenants/{tenant_id}/qris/templates", s.public(s.listTenantQRSTemplates))
	m.HandleFunc("POST /v1/tenants/{tenant_id}/qris/dynamic", s.public(s.createTenantDynamicQRIS))
	m.HandleFunc("GET /v1/invoices/{invoice_id}", s.public(s.getInvoice))
	m.HandleFunc("POST /v1/invoices/{invoice_id}/check", s.public(s.checkInvoice))
	m.HandleFunc("GET /v1/invoices/{invoice_id}/qr", s.public(s.qr))
	m.HandleFunc("GET /admin/tenants", s.admin("viewer", s.listTenants))
	m.HandleFunc("POST /admin/tenants", s.admin("super_admin", s.createTenant))
	m.HandleFunc("PUT /admin/tenants/{id}", s.admin("super_admin", s.updateTenant))
	m.HandleFunc("DELETE /admin/tenants/{id}", s.admin("super_admin", s.deleteTenant))
	m.HandleFunc("GET /admin/tenants/{id}/credentials", s.admin("super_admin", s.revealTenantCredential))
	m.HandleFunc("POST /admin/tenants/{id}/credentials/rotate", s.admin("super_admin", s.rotateTenantCredential))
	m.HandleFunc("POST /admin/tenants/{id}/webhook-secret/rotate", s.admin("super_admin", s.rotateTenantWebhookSecret))
	m.HandleFunc("GET /admin/merchant-ids", s.admin("viewer", s.listMerchantIDs))
	m.HandleFunc("POST /admin/merchant-ids", s.admin("operator", s.createMerchantID))
	m.HandleFunc("GET /admin/merchant-ids/{id}/connection", s.admin("viewer", s.getMerchantConnection))
	m.HandleFunc("PUT /admin/merchant-ids/{id}/connection/credentials", s.admin("operator", s.updateMerchantBrowserCredential))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/session", s.admin("operator", s.importMerchantSession))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/har", s.admin("operator", s.importMerchantHAR))
	m.HandleFunc("POST /admin/merchant-connections/har", s.admin("operator", s.importDefaultMerchantHAR))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/revoke", s.admin("operator", s.revokeMerchantSession))
	m.HandleFunc("POST /admin/merchant-ids/{id}/sync", s.admin("operator", s.requestMerchantSync))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/manual-login", s.admin("operator", s.startManualMerchantLogin))
	m.HandleFunc("GET /admin/merchant-ids/{id}/tariff", s.admin("viewer", s.getTariff))
	m.HandleFunc("PUT /admin/merchant-ids/{id}/tariff", s.admin("operator", s.putTariff))
	m.HandleFunc("GET /admin/merchant-transactions", s.admin("viewer", s.listMerchantTransactions))
	m.HandleFunc("GET /admin/tenant-transactions", s.admin("viewer", s.listAdminTenantTransactions))
	m.HandleFunc("GET /admin/global-transactions", s.admin("viewer", s.listGlobalTransactions))
	m.HandleFunc("GET /admin/merchant-accounts", s.admin("viewer", s.listMerchants))
	m.HandleFunc("POST /admin/merchant-accounts", s.admin("operator", s.createMerchant))
	m.HandleFunc("GET /admin/merchant-accounts/{id}", s.admin("viewer", s.getMerchant))
	m.HandleFunc("PUT /admin/merchant-accounts/{id}", s.admin("operator", s.updateMerchant))
	m.HandleFunc("POST /admin/merchant-accounts/{id}/test-connection", s.admin("operator", s.testMerchant))
	m.HandleFunc("GET /admin/invoices", s.admin("viewer", s.listInvoices))
	m.HandleFunc("GET /admin/invoices/{id}", s.admin("viewer", s.getAdminInvoice))
	m.HandleFunc("GET /admin/dashboard", s.admin("viewer", s.dashboard))
	m.HandleFunc("GET /admin/health", s.admin("viewer", s.adminHealth))
	m.HandleFunc("GET /admin/qris-templates", s.admin("viewer", s.listQRSTemplates))
	m.HandleFunc("POST /admin/qris-templates", s.admin("operator", s.createQRSTemplate))
	m.HandleFunc("PUT /admin/qris-templates/{id}", s.admin("operator", s.updateQRSTemplate))
	m.HandleFunc("GET /admin/qris-templates/{id}/image", s.admin("viewer", s.qrisTemplateImage))
	m.HandleFunc("GET /admin/qris-test-payments", s.admin("viewer", s.listTestPayments))
	m.HandleFunc("POST /admin/qris-test-payments", s.admin("operator", s.createTestPayment))
	m.HandleFunc("GET /admin/qris-test-payments/{id}/qr", s.admin("viewer", s.testPaymentQR))
	m.HandleFunc("GET /admin/audit-events", s.admin("viewer", s.listAudit))
	m.HandleFunc("GET /admin/payment-themes", s.admin("viewer", s.listPaymentThemes))
	m.HandleFunc("POST /admin/payment-themes", s.admin("operator", s.createPaymentTheme))
	m.HandleFunc("GET /admin/payment-themes/{id}", s.admin("viewer", s.getPaymentTheme))
	m.HandleFunc("PUT /admin/payment-themes/{id}", s.admin("operator", s.updatePaymentTheme))
	m.HandleFunc("DELETE /admin/payment-themes/{id}", s.admin("super_admin", s.deletePaymentTheme))
	m.HandleFunc("POST /admin/payment-themes/{id}/publish", s.admin("operator", s.publishPaymentTheme))
	m.HandleFunc("POST /admin/payment-themes/{id}/duplicate", s.admin("operator", s.duplicatePaymentTheme))
	m.HandleFunc("POST /admin/payment-themes/{id}/set-default", s.admin("operator", s.setDefaultPaymentTheme))
	m.HandleFunc("POST /admin/payment-themes/{id}/archive", s.admin("operator", s.archivePaymentTheme))
	m.HandleFunc("GET /admin/payment-themes/{id}/preview", s.admin("viewer", s.getPaymentTheme))
	return securityHeaders(m)
}

// githubWebhook only accepts main push events and signals the host-side deployer.
// The API container never receives Docker or systemd privileges.
func (s Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	if s.WebhookSecret == "" || s.WebhookSignalPath == "" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid webhook body")
		return
	}
	signature := strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	_, _ = mac.Write(body)
	if len(signature) != sha256.Size*2 {
		problem(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}
	expected := fmt.Sprintf("%x", mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		problem(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}
	if r.Header.Get("X-GitHub-Event") != "push" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var payload struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Ref != "refs/heads/main" || len(payload.After) != 40 {
		problem(w, http.StatusBadRequest, "main push payload required")
		return
	}
	tmp := s.WebhookSignalPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(payload.After+"\n"), 0600); err != nil {
		problem(w, http.StatusServiceUnavailable, "deployment signal unavailable")
		return
	}
	if err := os.Rename(tmp, s.WebhookSignalPath); err != nil {
		_ = os.Remove(tmp)
		problem(w, http.StatusServiceUnavailable, "deployment signal unavailable")
		return
	}
	respond(w, map[string]string{"status": "accepted", "commit": payload.After}, nil)
}
func (s Server) listMerchantIDs(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.ListMerchantIDs(r.Context())
	respond(w, v, err)
}
func (s Server) createMerchantID(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var in struct {
		ID                    string          `json:"id"`
		InteractiveMerchantID string          `json:"interactive_merchant_id"`
		Name                  string          `json:"name"`
		Credential            json.RawMessage `json:"credential"`
		BrowserEmail          string          `json:"browser_email"`
		BrowserPassword       string          `json:"browser_password"`
		TenantIDs             []string        `json:"tenant_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID == "" || in.InteractiveMerchantID == "" || in.Name == "" {
		problem(w, 400, "id, interactive_merchant_id and name are required")
		return
	}
	credential := in.Credential
	if len(credential) == 0 {
		// Browser-only connections can reconcile portal history without the public API.
		credential = json.RawMessage(`{}`)
	} else {
		var cfg qrisprovider.OpenAPIConfig
		if err := json.Unmarshal(credential, &cfg); err != nil {
			problem(w, 400, "invalid InterActive QRIS credential")
			return
		}
		if cfg.MerchantID != in.InteractiveMerchantID {
			problem(w, 400, "credential merchant_id must match interactive_merchant_id")
			return
		}
		if _, err := qrisprovider.NewOpenAPI(cfg); err != nil {
			problem(w, 400, err.Error())
			return
		}
	}
	ciphertext, err := s.Cipher.Encrypt(credential)
	if err != nil {
		problem(w, 500, "encrypt credential failed")
		return
	}
	if (in.BrowserEmail == "") != (in.BrowserPassword == "") {
		problem(w, 400, "browser_email and browser_password must be provided together")
		return
	}
	browserCredentialCiphertext := ""
	if in.BrowserEmail != "" {
		browserCredential, marshalErr := json.Marshal(struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}{Email: in.BrowserEmail, Password: in.BrowserPassword})
		if marshalErr != nil {
			problem(w, 500, "encode browser credential failed")
			return
		}
		browserCredentialCiphertext, err = s.Cipher.Encrypt(browserCredential)
		if err != nil {
			problem(w, 500, "encrypt browser credential failed")
			return
		}
	}
	v := domain.MerchantID{ID: in.ID, InteractiveMerchantID: in.InteractiveMerchantID, Name: in.Name, CredentialCiphertext: ciphertext, Active: true, CreatedAt: time.Now().UTC()}
	if err = s.Repo.CreateMerchantID(r.Context(), v); err != nil {
		problem(w, 400, "create merchant ID failed")
		return
	}
	for _, tenantID := range in.TenantIDs {
		if err = s.Repo.AssignTenantMerchant(r.Context(), tenantID, v.ID); err != nil {
			problem(w, 400, "assign tenant failed")
			return
		}
	}
	_ = s.Repo.UpsertMerchantConnection(r.Context(), domain.MerchantConnection{MerchantID: v.ID, BrowserCredentialCiphertext: browserCredentialCiphertext, Status: domain.ConnectionDisconnected, UpdatedAt: time.Now().UTC()})
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: v.ID + "-created", Actor: "admin", Action: "merchant_id.created", ResourceType: "merchant_id", ResourceID: v.ID, CreatedAt: time.Now().UTC()})
	write(w, 201, v)
}
func (s Server) getMerchantConnection(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.MerchantConnection(r.Context(), r.PathValue("id"))
	respond(w, v, err)
}
func (s Server) updateMerchantBrowserCredential(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Email) == "" || in.Password == "" {
		problem(w, 400, "email and password are required")
		return
	}
	connection, err := s.Repo.MerchantConnection(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	raw, _ := json.Marshal(map[string]string{"email": strings.TrimSpace(in.Email), "password": in.Password})
	connection.BrowserCredentialCiphertext, err = s.Cipher.Encrypt(raw)
	if err != nil {
		problem(w, 500, "encrypt browser credential failed")
		return
	}
	connection.Status = domain.ConnectionReconnectRequired
	connection.LastError = "Browser connection queued"
	connection.UpdatedAt = time.Unix(0, 0).UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), connection); err != nil {
		problem(w, 500, "update browser credential failed")
		return
	}
	write(w, http.StatusAccepted, map[string]string{"status": "queued"})
}
func (s Server) importMerchantSession(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	type importedCookie struct {
		Name     string `json:"name"`
		Value    string `json:"value"`
		Domain   string `json:"domain"`
		Path     string `json:"path"`
		Secure   bool   `json:"secure"`
		HTTPOnly bool   `json:"httpOnly"`
		SameSite string `json:"sameSite"`
		HostOnly bool   `json:"hostOnly"`
		Session  bool   `json:"session"`
		StoreID  any    `json:"storeId"`
	}
	type checkerCookie struct {
		Name     string `json:"name"`
		Value    string `json:"value"`
		Domain   string `json:"domain"`
		Path     string `json:"path,omitempty"`
		Secure   bool   `json:"secure"`
		HTTPOnly bool   `json:"httpOnly"`
		SameSite string `json:"sameSite,omitempty"`
	}
	var cookies []importedCookie
	if !decode(w, r, &cookies) {
		return
	}
	valid := false
	checkerCookies := make([]checkerCookie, 0, len(cookies))
	for _, c := range cookies {
		if c.Name != "" && c.Value != "" && strings.Contains(strings.ToLower(c.Domain), "interactive.co.id") {
			valid = true
		}
		sameSite := strings.ToLower(c.SameSite)
		if sameSite == "lax" {
			sameSite = "Lax"
		}
		if sameSite == "strict" {
			sameSite = "Strict"
		}
		if sameSite == "none" {
			sameSite = "None"
		}
		checkerCookies = append(checkerCookies, checkerCookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HTTPOnly, SameSite: sameSite})
	}
	if !valid {
		problem(w, 400, "JSON cookies for interactive.co.id are required")
		return
	}
	raw, _ := json.Marshal(checkerCookies)
	encrypted, err := s.Cipher.Encrypt(raw)
	if err != nil {
		problem(w, 500, "encrypt session failed")
		return
	}
	now := time.Now().UTC()
	v := domain.MerchantConnection{MerchantID: r.PathValue("id"), SessionCiphertext: encrypted, Status: domain.ConnectionConnected, UpdatedAt: now}
	if previous, lookupErr := s.Repo.MerchantConnection(r.Context(), v.MerchantID); lookupErr == nil {
		v.LastSyncedAt = previous.LastSyncedAt
	}
	if err = s.Repo.UpsertMerchantConnection(r.Context(), v); err != nil {
		problem(w, 400, "save session failed")
		return
	}
	write(w, 200, v)
}
func (s Server) importMerchantHAR(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	s.importHAR(w, r, r.PathValue("id"), false)
}

// importDefaultMerchantHAR keeps the console's first browser connection one-click.
// A HAR does not expose a merchant identifier, so subsequent uploads update this
// connection until an authenticated browser session provides live portal data.
func (s Server) importDefaultMerchantHAR(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	const browserMerchantID = "interactive-browser"
	if _, err := s.Repo.MerchantID(r.Context(), browserMerchantID); err != nil {
		ciphertext, encryptErr := s.Cipher.Encrypt([]byte(`{}`))
		if encryptErr != nil {
			problem(w, 500, "initialize browser connection failed")
			return
		}
		merchant := domain.MerchantID{ID: browserMerchantID, InteractiveMerchantID: "browser-session", Name: "InterActive QRIS browser", CredentialCiphertext: ciphertext, Active: true, CreatedAt: time.Now().UTC()}
		if createErr := s.Repo.CreateMerchantID(r.Context(), merchant); createErr != nil {
			problem(w, 400, "create browser connection failed")
			return
		}
		_ = s.Repo.UpsertMerchantConnection(r.Context(), domain.MerchantConnection{MerchantID: browserMerchantID, Status: domain.ConnectionDisconnected, UpdatedAt: time.Now().UTC()})
	}
	s.importHAR(w, r, browserMerchantID, true)
}

func (s Server) importHAR(w http.ResponseWriter, r *http.Request, merchantID string, includeMerchant bool) {
	// Chrome HAR exports can include embedded response bodies and exceed 12 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	var har struct {
		Log struct {
			Pages []struct {
				Title string `json:"title"`
			} `json:"pages"`
			Entries []struct {
				Request struct {
					URL string `json:"url"`
				} `json:"request"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.NewDecoder(r.Body).Decode(&har); err != nil {
		problem(w, 400, "valid HAR JSON is required")
		return
	}
	if len(har.Log.Entries) == 0 {
		problem(w, 400, "HAR contains no entries")
		return
	}
	hasInteractive, hasHistory := false, false
	for _, entry := range har.Log.Entries {
		lower := strings.ToLower(entry.Request.URL)
		hasInteractive = hasInteractive || strings.Contains(lower, "merchant.qris.interactive.co.id")
		hasHistory = hasHistory || strings.Contains(lower, "historytrx.php")
	}
	if !hasInteractive {
		problem(w, 400, "HAR must target merchant.qris.interactive.co.id")
		return
	}
	v, err := s.Repo.MerchantConnection(r.Context(), merchantID)
	if err != nil {
		notFound(w, err)
		return
	}
	v.Status, v.LastError, v.UpdatedAt = domain.ConnectionReconnectRequired, "HAR imported; upload JSON cookies to activate the checker", time.Now().UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), v); err != nil {
		problem(w, 500, "save HAR import failed")
		return
	}
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: merchantID + "-har-" + newID(), Actor: "admin", Action: "merchant_connection.har_imported", ResourceType: "merchant_id", ResourceID: merchantID, Metadata: map[string]any{"entries": len(har.Log.Entries), "pages": len(har.Log.Pages), "history_route": hasHistory}, CreatedAt: time.Now().UTC()})
	response := map[string]any{"entries": len(har.Log.Entries), "pages": len(har.Log.Pages), "history_route": hasHistory, "session_required": true, "connection": v}
	if includeMerchant {
		response["merchant_id"] = merchantID
	}
	write(w, 200, response)
}
func (s Server) revokeMerchantSession(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	now := time.Now().UTC()
	v := domain.MerchantConnection{MerchantID: r.PathValue("id"), Status: domain.ConnectionDisconnected, UpdatedAt: now}
	if err := s.Repo.UpsertMerchantConnection(r.Context(), v); err != nil {
		problem(w, 400, "revoke session failed")
		return
	}
	write(w, 200, v)
}
func (s Server) startManualMerchantLogin(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	if s.ManualLogin == nil {
		problem(w, http.StatusServiceUnavailable, "manual browser login is not configured")
		return
	}
	connection, err := s.Repo.MerchantConnection(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	if connection.LastError == "Manual browser login queued" || connection.LastError == "Manual browser login in progress" {
		write(w, http.StatusAccepted, map[string]string{"status": "manual_login_queued"})
		return
	}
	connection.Status = domain.ConnectionReconnectRequired
	connection.LastError = "Manual browser login queued"
	connection.LastSyncedAt = nil
	connection.UpdatedAt = time.Now().UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), connection); err != nil {
		problem(w, http.StatusInternalServerError, "mark manual browser login failed")
		return
	}
	job, _, err := s.enqueueBrowserJob(r.Context(), "manual_login", connection.MerchantID, 100)
	if err != nil {
		problem(w, http.StatusInternalServerError, "queue manual browser login failed")
		return
	}
	write(w, http.StatusAccepted, map[string]string{"status": "manual_login_queued", "job_id": job.ID})
}

func (s Server) requestMerchantSync(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.MerchantConnection(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	if v.Status != domain.ConnectionConnected {
		v.Status = domain.ConnectionReconnectRequired
	}
	v.LastSyncedAt = nil
	v.LastError = "Browser connection queued"
	v.UpdatedAt = time.Now().UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), v); err != nil {
		problem(w, 500, "queue sync failed")
		return
	}
	job, _, err := s.enqueueBrowserJob(r.Context(), "merchant_sync", v.MerchantID, 80)
	if err != nil {
		problem(w, http.StatusInternalServerError, "queue sync failed")
		return
	}
	write(w, 202, map[string]string{"status": "queued", "job_id": job.ID})
}
func (s Server) refreshTenantTransactions(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	if tenant.MerchantID == "" {
		problem(w, http.StatusConflict, "tenant is not linked to a Merchant ID")
		return
	}
	connection, err := s.Repo.MerchantConnection(r.Context(), tenant.MerchantID)
	if err != nil {
		problem(w, http.StatusConflict, "merchant browser connection is not configured")
		return
	}
	if connection.Status != domain.ConnectionConnected {
		connection.Status = domain.ConnectionReconnectRequired
	}
	connection.LastSyncedAt = nil
	connection.LastError = "Tenant requested transaction refresh"
	connection.UpdatedAt = time.Now().UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), connection); err != nil {
		problem(w, http.StatusInternalServerError, "queue refresh failed")
		return
	}
	job, _, err := s.enqueueBrowserJob(r.Context(), "merchant_sync", tenant.MerchantID, 80)
	if err != nil {
		problem(w, http.StatusInternalServerError, "queue refresh failed")
		return
	}
	write(w, http.StatusAccepted, map[string]any{
		"status": "queued", "tenant_id": tenant.ID, "merchant_id": tenant.MerchantID,
		"poll_url": "/v1/tenants/" + tenant.ID + "/transactions", "job_id": job.ID,
	})
}
func (s Server) listTenantTransactions(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	limit, ok := listLimit(w, r)
	if !ok {
		return
	}
	transactions, err := s.Repo.ListPortalTransactions(r.Context(), tenant.MerchantID, tenant.ID, limit)
	if err != nil {
		respond(w, nil, err)
		return
	}
	payments, err := s.Repo.ListTenantTestPayments(r.Context(), tenant.ID, limit)
	if err != nil {
		respond(w, nil, err)
		return
	}
	type transactionItem struct {
		value   any
		eventAt time.Time
	}
	merged := make([]transactionItem, 0, len(transactions)+len(payments))
	for _, transaction := range transactions {
		merged = append(merged, transactionItem{value: transaction, eventAt: transaction.PaidAt})
	}
	for _, payment := range payments {
		merged = append(merged, transactionItem{value: tenantPaymentLog(payment), eventAt: payment.CreatedAt})
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].eventAt.After(merged[j].eventAt) })
	if len(merged) > limit {
		merged = merged[:limit]
	}
	items := make([]any, 0, len(merged))
	for _, item := range merged {
		items = append(items, item.value)
	}
	respond(w, items, nil)
}

type tenantQRISResponse struct {
	domain.TestPayment
	TemplateID       string `json:"template_id"`
	Currency         string `json:"currency"`
	RequestedAmount  int64  `json:"requested_amount"`
	QRPayload        string `json:"qr_payload,omitempty"`
	QRPNGBase64      string `json:"qr_png_base64,omitempty"`
	StatusURL        string `json:"status_url"`
	QRURL            string `json:"qr_url"`
	PollAfterSeconds int    `json:"poll_after_seconds,omitempty"`
}

func pollAfterSeconds(payment domain.TestPayment, now time.Time) int {
	if payment.Status != domain.InvoicePending || payment.NextCheckAt == nil {
		return 0
	}
	seconds := int(payment.NextCheckAt.Sub(now).Round(time.Second).Seconds())
	if payment.NextCheckAt.After(now) && seconds < 1 {
		seconds = 1
	}
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

func setPollHeaders(w http.ResponseWriter, payment domain.TestPayment, now time.Time) {
	if delay := pollAfterSeconds(payment, now); delay > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(delay))
	}
}

func tenantPaymentLog(payment domain.TestPayment) map[string]any {
	return map[string]any{
		"id": payment.ID, "tenant_id": payment.TenantID, "merchant_id": payment.MerchantID,
		"reference": payment.UniqueCode, "amount": payment.Amount, "requested_amount": payment.Amount,
		"payable_amount": payment.PayableAmount, "unique_amount_code": payment.UniqueAmountCode, "status": payment.Status,
		"source": "tenant_qris", "request_source": payment.RequestSource,
		"match_confidence": payment.MatchConfidence, "created_at": payment.CreatedAt,
		"expires_at": payment.ExpiresAt, "last_checked_at": payment.LastCheckedAt,
		"next_check_at": payment.NextCheckAt, "check_count": payment.CheckCount,
		"matched_transaction_id": payment.MatchedTransactionID,
	}
}

func (s Server) createTenantQRISTransaction(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	s.createTenantQRIS(w, r, tenant)
}

func (s Server) createTenantDynamicQRIS(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	s.createTenantQRIS(w, r, tenant)
}

func (s Server) listTenantQRSTemplates(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	templates, err := s.Repo.ListQRISTemplates(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	available := make([]domain.QRISTemplate, 0, len(templates))
	for _, template := range templates {
		if template.Active && template.StaticToDynamic && tenantCanAccessQRSTemplate(template, tenant.ID) {
			template.StaticPayload = ""
			template.ImageData = nil
			available = append(available, template)
		}
	}
	write(w, http.StatusOK, available)
}

func tenantCanAccessQRSTemplate(template domain.QRISTemplate, tenantID string) bool {
	accessScope := template.AccessScope
	if accessScope == "" {
		if template.TenantID == "" {
			accessScope = "all_tenants"
		} else {
			accessScope = "selected_tenant"
		}
	}
	switch accessScope {
	case "all_tenants":
		return true
	case "selected_tenant":
		return template.TenantID == tenantID
	default:
		return false
	}
}

func (s Server) createTenantQRIS(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	var in struct {
		TemplateID       string `json:"template_id"`
		Amount           int64  `json:"amount"`
		IdempotencyKey   string `json:"idempotency_key"`
		ExpiresInSeconds int    `json:"expires_in_seconds"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.TemplateID = strings.TrimSpace(in.TemplateID)
	if in.TemplateID == "" {
		problem(w, http.StatusBadRequest, "template_id is required; retrieve it from GET /v1/tenants/"+tenant.ID+"/qris/templates")
		return
	}
	template, err := s.Repo.QRISTemplate(r.Context(), in.TemplateID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !tenantCanAccessQRSTemplate(template, tenant.ID)) {
		problem(w, http.StatusNotFound, "QRIS template not found or not accessible; retrieve template_id from GET /v1/tenants/"+tenant.ID+"/qris/templates")
		return
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "load QRIS template failed")
		return
	}
	if !template.Active || !template.StaticToDynamic {
		problem(w, http.StatusConflict, "QRIS template is not enabled for dynamic tenant requests; retrieve an available template_id from GET /v1/tenants/"+tenant.ID+"/qris/templates")
		return
	}
	if tenant.MerchantID == "" {
		problem(w, http.StatusConflict, "tenant is not linked to a Merchant ID")
		return
	}
	if in.Amount <= 0 || in.Amount > 100_000_000 {
		problem(w, http.StatusBadRequest, "amount must be between 1 and 100000000")
		return
	}
	if tenant.UseUniqueAmountCode && in.Amount > 99_999_901 {
		problem(w, http.StatusBadRequest, "amount must not exceed 99999901 when unique amount codes are enabled")
		return
	}
	now := time.Now().UTC()
	maxRequests := template.MaxRequestsPM
	if maxRequests < 1 {
		maxRequests = 60
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(in.IdempotencyKey)
	}
	if key == "" {
		key = newID()
	}
	if len(key) > 200 {
		problem(w, http.StatusBadRequest, "idempotency key is too long")
		return
	}
	expiresIn := 30 * 60
	if in.ExpiresInSeconds != 0 {
		if in.ExpiresInSeconds < 60 || in.ExpiresInSeconds > 30*60 {
			problem(w, http.StatusBadRequest, "expires_in_seconds must be between 60 and 1800")
			return
		}
		expiresIn = in.ExpiresInSeconds
	}
	uniqueCode := newUniqueCode()
	codes := []int64{0}
	if tenant.UseUniqueAmountCode {
		order := s.UniqueAmountCodeOrder
		if order == nil {
			order = secureUniqueAmountCodeOrder
		}
		codes, err = order()
		if err != nil {
			problem(w, http.StatusInternalServerError, "generate unique amount code failed")
			return
		}
	}
	var stored domain.TestPayment
	var created, allowed bool
	var retryAfter int
	for _, amountCode := range codes {
		payableAmount := in.Amount + amountCode
		payload, convertErr := qrisservice.Convert(template.StaticPayload, payableAmount)
		if convertErr != nil {
			problem(w, http.StatusBadRequest, convertErr.Error())
			return
		}
		payload, convertErr = qrisservice.WithBillNumber(payload, uniqueCode)
		if convertErr != nil {
			problem(w, http.StatusBadRequest, convertErr.Error())
			return
		}
		payment := domain.TestPayment{
			ID: newID(), IdempotencyKey: key, QRISTemplateID: template.ID, MerchantID: tenant.MerchantID, TenantID: tenant.ID,
			Amount: in.Amount, PayableAmount: payableAmount, UniqueAmountCode: amountCode,
			DynamicPayload: payload, UniqueCode: uniqueCode, Status: domain.InvoicePending,
			RequestSource: "tenant_api", MatchConfidence: "waiting_first_check", SandboxMode: tenant.SandboxMode, CreatedAt: now, UpdatedAt: now,
			ExpiresAt: now.Add(time.Duration(expiresIn) * time.Second),
		}
		next := now.Add(15 * time.Second)
		payment.NextCheckAt = &next
		stored, created, allowed, retryAfter, err = s.Repo.CreateTenantTestPayment(r.Context(), payment, now, maxRequests)
		if errors.Is(err, store.ErrUniqueAmountUnavailable) {
			continue
		}
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "idempotency key was reused with different transaction data")
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, "save QRIS transaction failed")
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			problem(w, http.StatusTooManyRequests, "QRIS request limit exceeded")
			return
		}
		break
	}
	if stored.ID == "" {
		if tenant.UseUniqueAmountCode {
			w.Header().Set("Retry-After", "30")
			problem(w, http.StatusTooManyRequests, "all unique amount codes are currently reserved")
			return
		}
		problem(w, http.StatusConflict, "another pending transaction uses the same merchant and amount")
		return
	}
	png, err := qrisservice.PNG(stored.DynamicPayload)
	if err != nil {
		problem(w, http.StatusInternalServerError, "QRIS image generation failed")
		return
	}
	if created {
		s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), TenantID: tenant.ID, Actor: "tenant_api", Action: "qris.transaction_created", ResourceType: "test_payment", ResourceID: stored.ID, Metadata: map[string]any{"requested_amount": stored.Amount, "payable_amount": stored.PayableAmount, "unique_amount_code": stored.UniqueAmountCode}, CreatedAt: now})
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	setPollHeaders(w, stored, now)
	write(w, status, tenantQRISResponse{TestPayment: stored, TemplateID: stored.QRISTemplateID, Currency: "IDR", RequestedAmount: stored.Amount, QRPayload: stored.DynamicPayload, QRPNGBase64: base64.StdEncoding.EncodeToString(png), StatusURL: "/v1/tenants/" + tenant.ID + "/transactions/qris/" + stored.ID, QRURL: "/v1/tenants/" + tenant.ID + "/transactions/qris/" + stored.ID + "/qr", PollAfterSeconds: pollAfterSeconds(stored, now)})
}

func (s Server) getTenantQRISTransaction(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	_, _ = s.Repo.ExpirePendingTestPayments(r.Context(), time.Now().UTC())
	payment, err := s.Repo.TestPaymentForTenant(r.Context(), tenant.ID, r.PathValue("transaction_id"))
	if err != nil {
		notFound(w, err)
		return
	}
	now := time.Now().UTC()
	response := tenantQRISResponse{TestPayment: payment, TemplateID: payment.QRISTemplateID, Currency: "IDR", RequestedAmount: payment.Amount, StatusURL: r.URL.Path, QRURL: r.URL.Path + "/qr", PollAfterSeconds: pollAfterSeconds(payment, now)}
	if payment.Status == domain.InvoicePending {
		png, pngErr := qrisservice.PNG(payment.DynamicPayload)
		if pngErr != nil {
			problem(w, http.StatusInternalServerError, "QRIS image generation failed")
			return
		}
		response.QRPayload = payment.DynamicPayload
		response.QRPNGBase64 = base64.StdEncoding.EncodeToString(png)
	} else {
		response.DynamicPayload = ""
	}
	setPollHeaders(w, payment, now)
	write(w, http.StatusOK, response)
}

func (s Server) cancelTenantQRISTransaction(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	now := time.Now().UTC()
	id := r.PathValue("transaction_id")
	audit := domain.AuditEvent{ID: newID(), TenantID: tenant.ID, Actor: "tenant_api", Action: "qris.transaction_cancelled", ResourceType: "test_payment", ResourceID: id, CreatedAt: now}
	payment, _, err := s.Repo.CancelPendingTestPayment(r.Context(), tenant.ID, id, now, audit)
	if errors.Is(err, store.ErrNotFound) {
		problem(w, http.StatusNotFound, "not found")
		return
	}
	response := tenantQRISResponse{TestPayment: payment, TemplateID: payment.QRISTemplateID, Currency: "IDR", RequestedAmount: payment.Amount, StatusURL: strings.TrimSuffix(r.URL.Path, "/cancel"), QRURL: strings.TrimSuffix(r.URL.Path, "/cancel") + "/qr"}
	response.DynamicPayload = ""
	if errors.Is(err, store.ErrConflict) {
		write(w, http.StatusConflict, map[string]any{"error": "QRIS transaction is already terminal", "transaction": response})
		return
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "cancel QRIS transaction failed")
		return
	}
	write(w, http.StatusOK, response)
}

func (s Server) getTenantQRISTransactionQR(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	_, _ = s.Repo.ExpirePendingTestPayments(r.Context(), time.Now().UTC())
	payment, err := s.Repo.TestPaymentForTenant(r.Context(), tenant.ID, r.PathValue("transaction_id"))
	if err != nil {
		notFound(w, err)
		return
	}
	if payment.Status != domain.InvoicePending {
		problem(w, http.StatusGone, "QRIS transaction is no longer payable")
		return
	}
	png, err := qrisservice.PNG(payment.DynamicPayload)
	if err != nil {
		problem(w, http.StatusInternalServerError, "QRIS image generation failed")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}
func (s Server) getTariff(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.Tariff(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		v = domain.Tariff{MerchantID: r.PathValue("id"), Active: true}
		err = nil
	}
	respond(w, v, err)
}
func (s Server) putTariff(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var v domain.Tariff
	if !decode(w, r, &v) {
		return
	}
	v.MerchantID = r.PathValue("id")
	if v.BasisPoints < 0 || v.FixedFee < 0 {
		problem(w, 400, "tariff values must be non-negative")
		return
	}
	v.UpdatedAt = time.Now().UTC()
	if err := s.Repo.UpsertTariff(r.Context(), v); err != nil {
		problem(w, 500, "save tariff failed")
		return
	}
	write(w, 200, v)
}
func (s Server) listMerchantTransactions(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	limit, ok := listLimit(w, r)
	if !ok {
		return
	}
	v, err := s.Repo.ListPortalTransactions(r.Context(), r.URL.Query().Get("merchant_id"), r.URL.Query().Get("tenant_id"), limit)
	respond(w, v, err)
}
func (s Server) listAdminTenantTransactions(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	limit, ok := listLimit(w, r)
	if !ok {
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	invoices, err := s.Repo.ListInvoices(r.Context(), tenantID, 500)
	if err != nil {
		respond(w, nil, err)
		return
	}
	_, _ = s.Repo.ExpirePendingTestPayments(r.Context(), time.Now().UTC())
	payments, err := s.Repo.ListTenantTestPayments(r.Context(), tenantID, 500)
	if err != nil {
		respond(w, nil, err)
		return
	}
	items := make([]domain.TenantTransaction, 0, len(invoices)+len(payments))
	for _, invoice := range invoices {
		requestedAmount := invoice.RequestedAmount
		if requestedAmount <= 0 {
			requestedAmount = invoice.Amount
		}
		items = append(items, domain.TenantTransaction{
			ID: invoice.ID, TenantID: invoice.TenantID, MerchantID: invoice.MerchantAccountID,
			Kind: "invoice", Mode: transactionMode(invoice.SandboxMode), RequestSource: "tenant_invoice_api",
			IdempotencyKey: invoice.IdempotencyKey, Amount: requestedAmount, PayableAmount: invoice.Amount,
			UniqueAmountCode: invoice.UniqueAmountCode, Currency: invoice.Currency,
			Status: invoice.Status, ProviderReference: invoice.ProviderReference, CreatedAt: invoice.CreatedAt,
			UpdatedAt: invoice.UpdatedAt, ExpiresAt: invoice.ExpiresAt, LastCheckedAt: invoice.LastCheckedAt,
			CheckCount: invoice.CheckCount,
		})
	}
	for _, payment := range payments {
		items = append(items, domain.TenantTransaction{
			ID: payment.ID, TenantID: payment.TenantID, MerchantID: payment.MerchantID,
			Kind: "qris", Mode: transactionMode(payment.SandboxMode), RequestSource: payment.RequestSource,
			IdempotencyKey: payment.IdempotencyKey, Amount: payment.Amount, PayableAmount: payment.PayableAmount,
			UniqueAmountCode: payment.UniqueAmountCode, Currency: "IDR",
			Status: payment.Status, Validation: payment.MatchConfidence,
			MatchedTransactionID: payment.MatchedTransactionID, CreatedAt: payment.CreatedAt,
			UpdatedAt: payment.UpdatedAt, ExpiresAt: payment.ExpiresAt,
			LastCheckedAt: payment.LastCheckedAt, CheckCount: payment.CheckCount,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	respond(w, items, nil)
}

func transactionMode(sandbox bool) string {
	if sandbox {
		return "sandbox"
	}
	return "production"
}
func (s Server) listGlobalTransactions(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	limit, ok := listLimit(w, r)
	if !ok {
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	transactions, err := s.Repo.ListPortalTransactions(r.Context(), "", tenantID, 500)
	if err != nil {
		respond(w, nil, err)
		return
	}
	_, _ = s.Repo.ExpirePendingTestPayments(r.Context(), time.Now().UTC())
	testPayments, err := s.Repo.ListTestPayments(r.Context(), 500)
	if err != nil {
		respond(w, nil, err)
		return
	}
	items := make([]domain.GlobalTransactionLog, 0, len(transactions)+len(testPayments))
	for _, transaction := range transactions {
		items = append(items, domain.GlobalTransactionLog{
			ID: transaction.ID, EventType: "merchant_transaction", MerchantID: transaction.MerchantID,
			TenantID: transaction.TenantID, Reference: transaction.Reference, Amount: transaction.Amount,
			Status: transaction.Status, EventAt: transaction.PaidAt, Source: transaction.Source,
			RequestSource: "merchant_history_sync", Validation: transaction.MatchConfidence,
			InvoiceID: transaction.InvoiceID,
		})
	}
	for _, payment := range testPayments {
		if tenantID != "" && payment.TenantID != tenantID {
			continue
		}
		expiresAt := payment.ExpiresAt
		var nextCheckAt *time.Time
		if payment.Status == domain.InvoicePending {
			nextCheckAt = payment.NextCheckAt
		}
		items = append(items, domain.GlobalTransactionLog{
			ID: "test-" + payment.ID, EventType: "qris_test_check", MerchantID: payment.MerchantID,
			TenantID: payment.TenantID, Reference: payment.ID, Amount: payment.Amount,
			Status: string(payment.Status), EventAt: payment.CreatedAt, Source: "qris_test",
			RequestSource: payment.RequestSource, Validation: payment.MatchConfidence,
			ExpiresAt: &expiresAt, LastCheckedAt: payment.LastCheckedAt, NextCheckAt: nextCheckAt,
			CheckCount: payment.CheckCount, TestPaymentID: payment.ID,
			MatchedTransactionID: payment.MatchedTransactionID,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].EventAt.After(items[j].EventAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	respond(w, items, nil)
}

type handler func(http.ResponseWriter, *http.Request, domain.Tenant)

func (s Server) public(next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Add("Vary", "Origin")
		}
		key := r.Header.Get("X-API-Key")
		if key == "" {
			problem(w, 401, "missing API key")
			return
		}
		tenant, err := s.Repo.TenantByAPIKey(r.Context(), security.HashSecret(key))
		if err != nil {
			problem(w, 401, "invalid API key")
			return
		}
		pathTenant := r.PathValue("tenant_id")
		if pathTenant != "" && pathTenant != tenant.ID {
			problem(w, 404, "not found")
			return
		}
		if origin != "" {
			parsedOrigin, ok := parseOrigin(origin)
			if !ok || (!tenant.SandboxMode && parsedOrigin != integrationOrigin(tenant.SiteURL)) {
				problem(w, http.StatusForbidden, "invalid origin")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		next(w, r, tenant)
	}
}

// publicPayment protects token-based checkout endpoints from malformed origins
// while allowing a merchant-hosted checkout page to read its opaque session.
// The token itself remains the authorization boundary; tenant API-key checks
// continue to use public().
func (s Server) publicPayment(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, ok := parseOrigin(origin)
			if !ok {
				problem(w, http.StatusForbidden, "invalid origin")
				return
			}
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", parsed)
		}
		next(w, r)
	}
}

func tenantPreflight(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	w.Header().Add("Vary", "Origin")
	if _, ok := parseOrigin(origin); !ok {
		problem(w, http.StatusForbidden, "invalid origin")
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Idempotency-Key")
	w.WriteHeader(http.StatusNoContent)
}

func parseOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	return canonicalOrigin(u), true
}

func integrationOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return canonicalOrigin(u)
}

func canonicalOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	hostname := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return scheme + "://" + host
}
func (s Server) admin(min string, next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			problem(w, http.StatusUnauthorized, "missing admin token")
			return
		}
		role := s.AdminTokens[token]
		if rank(role) < rank(min) {
			problem(w, 403, "forbidden")
			return
		}
		next(w, r, domain.Tenant{})
	}
}
func rank(role string) int {
	switch role {
	case "viewer":
		return 1
	case "operator":
		return 2
	case "super_admin":
		return 3
	}
	return 0
}

type publicPaymentTheme struct {
	ID      string          `json:"id"`
	Version int             `json:"version"`
	Config  json.RawMessage `json:"config"`
}

type publicPaymentRedirect struct {
	SuccessURL string `json:"success_url,omitempty"`
	CancelURL  string `json:"cancel_url,omitempty"`
	FailedURL  string `json:"failed_url,omitempty"`
	ExpiredURL string `json:"expired_url,omitempty"`
}

// PublicPaymentSessionResponse is deliberately separate from domain.PaymentSession.
// It contains only checkout-safe invoice/session/theme fields.
type PublicPaymentSessionResponse struct {
	SessionID        string                 `json:"session_id"`
	InvoiceID        string                 `json:"invoice_id"`
	Status           string                 `json:"status"`
	PaymentStatus    string                 `json:"payment_status"`
	CheckoutURL      string                 `json:"checkout_url,omitempty"`
	Amount           int64                  `json:"amount"`
	RequestedAmount  int64                  `json:"requested_amount"`
	UniqueAmountCode int64                  `json:"unique_amount_code,omitempty"`
	QRISMerchantName string                 `json:"qris_merchant_name,omitempty"`
	QRISMerchantCity string                 `json:"qris_merchant_city,omitempty"`
	Currency         string                 `json:"currency"`
	Description      string                 `json:"description,omitempty"`
	QRPayload        string                 `json:"qr_payload,omitempty"`
	ExpiresAt        time.Time              `json:"expires_at"`
	ServerNow        time.Time              `json:"server_now"`
	Theme            *publicPaymentTheme    `json:"theme,omitempty"`
	Redirect         *publicPaymentRedirect `json:"redirect,omitempty"`
}

type createPaymentSessionRequest struct {
	InvoiceID  string `json:"invoice_id"`
	ThemeID    string `json:"theme_id"`
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
	FailedURL  string `json:"failed_url"`
	ExpiredURL string `json:"expired_url"`
}

type paymentSessionConflictResponse struct {
	Error   string                       `json:"error"`
	Session PublicPaymentSessionResponse `json:"session"`
}

func (s Server) paymentSessionService() gateway.PaymentSessionService {
	service := s.PaymentSessions
	if service.Repo == nil {
		service.Repo = s.Repo
	}
	if service.Now == nil {
		service.Now = s.Gateway.Now
	}
	return service
}

func (s Server) createPaymentSession(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	var in createPaymentSessionRequest
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.InvoiceID) == "" {
		paymentSessionProblem(w, http.StatusBadRequest, "invalid_request", "invoice_id is required")
		return
	}
	service := s.paymentSessionService()
	created, err := service.CreatePaymentSession(r.Context(), gateway.CreatePaymentSessionInput{
		TenantID: tenant.ID, InvoiceID: in.InvoiceID, ThemeID: in.ThemeID,
		SuccessURL: in.SuccessURL, CancelURL: in.CancelURL, FailedURL: in.FailedURL, ExpiredURL: in.ExpiredURL,
	})
	if err != nil {
		s.writePaymentSessionError(w, err)
		return
	}
	snapshot, err := service.Snapshot(r.Context(), created.PublicToken)
	if err != nil {
		paymentSessionProblem(w, http.StatusInternalServerError, "internal_error", "payment session could not be resolved")
		return
	}
	response := s.publicPaymentSessionResponse(snapshot)
	response.CheckoutURL = s.checkoutURL(created.PublicToken)
	write(w, http.StatusCreated, response)
}

func (s Server) getPaymentSession(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" || len(token) > 512 {
		paymentSessionProblem(w, http.StatusNotFound, "not_found", "payment session not found")
		return
	}
	snapshot, err := s.paymentSessionService().Snapshot(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			paymentSessionProblem(w, http.StatusNotFound, "not_found", "payment session not found")
			return
		}
		paymentSessionProblem(w, http.StatusConflict, "conflict", "payment session state could not be resolved")
		return
	}
	write(w, http.StatusOK, s.publicPaymentSessionResponse(snapshot))
}

func (s Server) cancelPaymentSession(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" || len(token) > 512 {
		paymentSessionProblem(w, http.StatusNotFound, "not_found", "payment session not found")
		return
	}
	snapshot, err := s.paymentSessionService().Cancel(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			paymentSessionProblem(w, http.StatusNotFound, "not_found", "payment session not found")
			return
		}
		if errors.Is(err, gateway.ErrPaymentSessionStateConflict) {
			write(w, http.StatusConflict, paymentSessionConflictResponse{Error: "payment session is already terminal", Session: s.publicPaymentSessionResponse(snapshot)})
			return
		}
		slog.Error("payment session cancellation failed", "error", err)
		paymentSessionProblem(w, http.StatusInternalServerError, "internal_error", "payment session cancellation failed")
		return
	}
	write(w, http.StatusOK, s.publicPaymentSessionResponse(snapshot))
}

func (s Server) publicPaymentSessionResponse(snapshot gateway.PaymentSessionSnapshot) PublicPaymentSessionResponse {
	response := PublicPaymentSessionResponse{
		SessionID: snapshot.Session.ID, InvoiceID: snapshot.Invoice.ID,
		Status:        publicPaymentSessionStatus(snapshot.Session.Status),
		PaymentStatus: strings.ToLower(string(snapshot.Invoice.Status)), Amount: snapshot.Invoice.Amount,
		RequestedAmount: snapshot.Invoice.RequestedAmount, UniqueAmountCode: snapshot.Invoice.UniqueAmountCode,
		QRISMerchantName: snapshot.Invoice.QRISMerchantName, QRISMerchantCity: snapshot.Invoice.QRISMerchantCity,
		Currency: snapshot.Invoice.Currency, Description: snapshot.Invoice.Description,
		ExpiresAt: snapshot.Session.ExpiresAt,
		ServerNow: time.Now().UTC(),
	}
	if snapshot.Invoice.Status == domain.InvoicePending {
		response.QRPayload = snapshot.Invoice.QRPayload
	}
	redirect := &publicPaymentRedirect{SuccessURL: snapshot.Session.SuccessURL, CancelURL: snapshot.Session.CancelURL, FailedURL: snapshot.Session.FailedURL, ExpiredURL: snapshot.Session.ExpiredURL}
	if redirect.SuccessURL != "" || redirect.CancelURL != "" || redirect.FailedURL != "" || redirect.ExpiredURL != "" {
		response.Redirect = redirect
	}
	if snapshot.Theme != nil {
		config := append(json.RawMessage(nil), snapshot.Theme.Config...)
		response.Theme = &publicPaymentTheme{ID: snapshot.Theme.ThemeID, Version: snapshot.Theme.Version, Config: config}
	}
	return response
}

func publicPaymentSessionStatus(status domain.PaymentSessionStatus) string {
	switch status {
	case domain.PaymentSessionOpen, domain.PaymentSessionPaymentPending:
		return "payment_pending"
	case domain.PaymentSessionPaid:
		return "paid"
	case domain.PaymentSessionCancelled:
		return "cancelled"
	case domain.PaymentSessionExpired:
		return "expired"
	case domain.PaymentSessionFailed:
		return "failed"
	case domain.PaymentSessionRedirecting:
		return "redirecting"
	case domain.PaymentSessionClosed:
		return "closed"
	default:
		return "unknown"
	}
}

func (s Server) checkoutURL(token string) string {
	base := strings.TrimRight(strings.TrimSpace(s.PublicPaymentBaseURL), "/")
	if base == "" {
		base = "https://pay.alpakyros.net"
	}
	return base + "/pay/" + token
}

func (s Server) writePaymentSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		paymentSessionProblem(w, http.StatusNotFound, "not_found", "invoice or payment session not found")
	case errors.Is(err, store.ErrConflict):
		paymentSessionProblem(w, http.StatusConflict, "conflict", "invoice or payment session state is not payable")
	case strings.Contains(err.Error(), "redirect URL"):
		paymentSessionProblem(w, http.StatusBadRequest, "invalid_redirect", "redirect URL is not allowed")
	default:
		paymentSessionProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
	}
}

func paymentSessionProblem(w http.ResponseWriter, status int, code, message string) {
	write(w, status, map[string]string{"error": message, "code": code})
}

func (s Server) createInvoice(w http.ResponseWriter, r *http.Request, t domain.Tenant) {
	var in struct {
		MerchantAccountID string `json:"merchant_account_id"`
		IdempotencyKey    string `json:"idempotency_key"`
		Currency          string `json:"currency"`
		Description       string `json:"description"`
		Amount            int64  `json:"amount"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.IdempotencyKey == "" {
		in.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	inv, created, err := s.Gateway.CreateInvoice(r.Context(), gateway.CreateInvoiceInput{TenantID: t.ID, MerchantAccountID: in.MerchantAccountID, IdempotencyKey: in.IdempotencyKey, Amount: in.Amount, Currency: in.Currency, Description: in.Description, SandboxMode: t.SandboxMode})
	if err != nil {
		if errors.Is(err, gateway.ErrIdempotencyConflict) {
			problem(w, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "create provider payment") {
			problem(w, http.StatusBadGateway, "provider payment creation failed")
			return
		}
		problem(w, 400, err.Error())
		return
	}
	code := 200
	if created {
		code = 201
	}
	write(w, code, inv)
}
func (s Server) getInvoice(w http.ResponseWriter, r *http.Request, t domain.Tenant) {
	inv, err := s.Gateway.Invoice(r.Context(), t.ID, r.PathValue("invoice_id"))
	if err != nil {
		notFound(w, err)
		return
	}
	write(w, 200, inv)
}
func (s Server) checkInvoice(w http.ResponseWriter, r *http.Request, t domain.Tenant) {
	inv, err := s.Gateway.Check(r.Context(), t.ID, r.PathValue("invoice_id"))
	if err != nil {
		if errors.Is(err, gateway.ErrCheckCooldown) {
			w.Header().Set("Retry-After", "60")
			problem(w, http.StatusTooManyRequests, err.Error())
			return
		}
		notFound(w, err)
		return
	}
	write(w, 200, inv)
}
func (s Server) qr(w http.ResponseWriter, r *http.Request, t domain.Tenant) {
	inv, err := s.Gateway.Invoice(r.Context(), t.ID, r.PathValue("invoice_id"))
	if err != nil {
		notFound(w, err)
		return
	}
	if inv.QRPayload == "" {
		problem(w, http.StatusConflict, "invoice QR is not ready")
		return
	}
	png, err := qrcode.Encode(inv.QRPayload, qrcode.Medium, 256)
	if err != nil {
		problem(w, 500, "QR generation failed")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}
func (s Server) listTenants(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.ListTenants(r.Context())
	for i := range v {
		v[i].APIKeyHash = ""
		v[i].APIKeyCiphertext = ""
	}
	respond(w, v, err)
}
func (s Server) createTenant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var in struct {
		Name                        string `json:"name"`
		MerchantID                  string `json:"merchant_id"`
		SiteURL                     string `json:"site_url"`
		CallbackURL                 string `json:"callback_url"`
		WebhookURL                  string `json:"webhook_url"`
		SandboxMode                 bool   `json:"sandbox_mode"`
		UseUniqueAmountCode         bool   `json:"use_unique_amount_code"`
		UniqueAmountCooldownMinutes int    `json:"unique_amount_cooldown_minutes"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.MerchantID == "" {
		problem(w, http.StatusBadRequest, "name and Merchant ID are required")
		return
	}
	if in.UniqueAmountCooldownMinutes == 0 {
		in.UniqueAmountCooldownMinutes = 30
	}
	if !validUniqueAmountCooldown(in.UniqueAmountCooldownMinutes) {
		problem(w, http.StatusBadRequest, "unique_amount_cooldown_minutes must be between 30 and 60")
		return
	}
	merchant, err := s.Repo.MerchantID(r.Context(), in.MerchantID)
	if err != nil || !merchant.Active {
		problem(w, http.StatusBadRequest, "active Merchant ID is required")
		return
	}
	for label, raw := range map[string]string{"site_url": in.SiteURL, "callback_url": in.CallbackURL, "webhook_url": in.WebhookURL} {
		if !validIntegrationURL(raw) {
			problem(w, http.StatusBadRequest, label+" must be an absolute HTTP(S) URL")
			return
		}
	}
	apiKey := newAPIKey()
	if s.Cipher == nil {
		problem(w, http.StatusInternalServerError, "tenant credential encryption is not configured")
		return
	}
	ciphertext, err := s.Cipher.Encrypt([]byte(apiKey))
	if err != nil {
		problem(w, http.StatusInternalServerError, "tenant credential encryption failed")
		return
	}
	v := domain.Tenant{ID: "tenant_" + newID()[:16], MerchantID: merchant.ID, Name: in.Name, SiteURL: strings.TrimSpace(in.SiteURL), CallbackURL: strings.TrimSpace(in.CallbackURL), WebhookURL: strings.TrimSpace(in.WebhookURL), SandboxMode: in.SandboxMode, UseUniqueAmountCode: in.UseUniqueAmountCode, UniqueAmountCooldownMinutes: in.UniqueAmountCooldownMinutes, APIKeyHash: security.HashSecret(apiKey), APIKeyCiphertext: ciphertext, APIKeyRecoverable: true, Active: true, CreatedAt: time.Now().UTC()}
	if err := s.Repo.CreateTenant(r.Context(), v); err != nil {
		problem(w, 500, "create tenant failed")
		return
	}
	v.APIKeyHash = ""
	v.APIKeyCiphertext = ""
	write(w, 201, map[string]any{"tenant": v, "api_key": apiKey, "api_key_visible_once": true})
}
func (s Server) updateTenant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	current, err := s.Repo.Tenant(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	var in struct {
		Name                        string `json:"name"`
		MerchantID                  string `json:"merchant_id"`
		SiteURL                     string `json:"site_url"`
		CallbackURL                 string `json:"callback_url"`
		WebhookURL                  string `json:"webhook_url"`
		SandboxMode                 bool   `json:"sandbox_mode"`
		UseUniqueAmountCode         *bool  `json:"use_unique_amount_code"`
		UniqueAmountCooldownMinutes *int   `json:"unique_amount_cooldown_minutes"`
		Active                      bool   `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.MerchantID == "" {
		problem(w, http.StatusBadRequest, "name and Merchant ID are required")
		return
	}
	if in.UniqueAmountCooldownMinutes != nil && !validUniqueAmountCooldown(*in.UniqueAmountCooldownMinutes) {
		problem(w, http.StatusBadRequest, "unique_amount_cooldown_minutes must be between 30 and 60")
		return
	}
	merchant, err := s.Repo.MerchantID(r.Context(), in.MerchantID)
	if err != nil || !merchant.Active {
		problem(w, http.StatusBadRequest, "active Merchant ID is required")
		return
	}
	for label, raw := range map[string]string{"site_url": in.SiteURL, "callback_url": in.CallbackURL, "webhook_url": in.WebhookURL} {
		if !validIntegrationURL(raw) {
			problem(w, http.StatusBadRequest, label+" must be an absolute HTTP(S) URL")
			return
		}
	}
	current.Name, current.MerchantID, current.SiteURL, current.CallbackURL, current.WebhookURL, current.SandboxMode, current.Active = in.Name, merchant.ID, strings.TrimSpace(in.SiteURL), strings.TrimSpace(in.CallbackURL), strings.TrimSpace(in.WebhookURL), in.SandboxMode, in.Active
	if in.UseUniqueAmountCode != nil {
		current.UseUniqueAmountCode = *in.UseUniqueAmountCode
	}
	if in.UniqueAmountCooldownMinutes != nil {
		current.UniqueAmountCooldownMinutes = *in.UniqueAmountCooldownMinutes
	}
	if err = s.Repo.UpdateTenant(r.Context(), current); err != nil {
		problem(w, http.StatusInternalServerError, "update tenant failed")
		return
	}
	current.APIKeyHash = ""
	current.APIKeyCiphertext = ""
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), Actor: "admin", Action: "tenant.updated", ResourceType: "tenant", ResourceID: current.ID, TenantID: current.ID, CreatedAt: time.Now().UTC()})
	write(w, http.StatusOK, current)
}

func (s Server) deleteTenant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	id := r.PathValue("id")
	audit := domain.AuditEvent{ID: newID(), TenantID: id, Actor: "admin", Action: "tenant.deleted", ResourceType: "tenant", ResourceID: id, CreatedAt: time.Now().UTC()}
	if err := s.Repo.DeleteTenant(r.Context(), id, audit); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "not found")
			return
		}
		problem(w, http.StatusInternalServerError, "delete tenant failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s Server) revealTenantCredential(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, err := s.Repo.Tenant(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	if tenant.APIKeyCiphertext == "" {
		write(w, http.StatusConflict, map[string]any{"error": "API key rotation required", "code": "api_key_rotation_required"})
		return
	}
	if s.Cipher == nil {
		problem(w, http.StatusInternalServerError, "tenant credential encryption is not configured")
		return
	}
	apiKey, err := s.Cipher.Decrypt(tenant.APIKeyCiphertext)
	if err != nil {
		problem(w, http.StatusInternalServerError, "tenant credential decryption failed")
		return
	}
	if err := s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), Actor: "admin", Action: "tenant.api_key_revealed", ResourceType: "tenant", ResourceID: tenant.ID, TenantID: tenant.ID, CreatedAt: time.Now().UTC()}); err != nil {
		problem(w, http.StatusInternalServerError, "audit tenant credential reveal failed")
		return
	}
	write(w, http.StatusOK, map[string]any{"tenant_id": tenant.ID, "api_key": string(apiKey)})
}

func (s Server) rotateTenantCredential(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, err := s.Repo.Tenant(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	if s.Cipher == nil {
		problem(w, http.StatusInternalServerError, "tenant credential encryption is not configured")
		return
	}
	apiKey := newAPIKey()
	expectedHash := tenant.APIKeyHash
	ciphertext, err := s.Cipher.Encrypt([]byte(apiKey))
	if err != nil {
		problem(w, http.StatusInternalServerError, "tenant credential encryption failed")
		return
	}
	tenant.APIKeyHash = security.HashSecret(apiKey)
	tenant.APIKeyCiphertext = ciphertext
	tenant.APIKeyRecoverable = true
	rotatedAt := time.Now().UTC()
	audit := domain.AuditEvent{ID: newID(), Actor: "admin", Action: "tenant.api_key_rotated", ResourceType: "tenant", ResourceID: tenant.ID, TenantID: tenant.ID, CreatedAt: rotatedAt}
	if err := s.Repo.RotateTenantAPIKey(r.Context(), tenant.ID, expectedHash, tenant.APIKeyHash, tenant.APIKeyCiphertext, audit); err != nil {
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "tenant credential changed; reload and retry")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "not found")
			return
		}
		problem(w, http.StatusInternalServerError, "rotate tenant credential failed")
		return
	}
	write(w, http.StatusOK, map[string]any{"tenant_id": tenant.ID, "api_key": apiKey, "rotated_at": rotatedAt})
}

func (s Server) rotateTenantWebhookSecret(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant, err := s.Repo.Tenant(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	if s.Cipher == nil {
		problem(w, http.StatusInternalServerError, "tenant credential encryption is not configured")
		return
	}
	secret, err := security.GenerateWebhookSecret()
	if err != nil {
		problem(w, http.StatusInternalServerError, "webhook secret generation failed")
		return
	}
	ciphertext, err := s.Cipher.Encrypt([]byte(secret))
	if err != nil {
		problem(w, http.StatusInternalServerError, "webhook secret encryption failed")
		return
	}
	now := time.Now().UTC()
	audit := domain.AuditEvent{ID: newID(), Actor: "admin", Action: "tenant.webhook_secret_rotated", ResourceType: "tenant", ResourceID: tenant.ID, TenantID: tenant.ID, CreatedAt: now}
	if err := s.Repo.RotateTenantWebhookSecret(r.Context(), tenant.ID, ciphertext, audit); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "not found")
			return
		}
		problem(w, http.StatusInternalServerError, "rotate webhook secret failed")
		return
	}
	write(w, http.StatusOK, map[string]any{"tenant_id": tenant.ID, "webhook_secret": secret, "rotated_at": now, "visible_once": true})
}
func (s Server) listMerchants(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.ListMerchantAccounts(r.Context(), r.URL.Query().Get("tenant_id"))
	for i := range v {
		v[i].CredentialCiphertext = ""
	}
	respond(w, v, err)
}
func (s Server) createMerchant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var in struct {
		ID         string          `json:"id"`
		TenantID   string          `json:"tenant_id"`
		Provider   string          `json:"provider"`
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID == "" || in.TenantID == "" || in.Name == "" || in.Provider != qrisprovider.InteractiveQRISProvider {
		problem(w, http.StatusBadRequest, "id, tenant_id, name and provider=interactive_qris are required")
		return
	}
	var cfg qrisprovider.OpenAPIConfig
	if err := json.Unmarshal(in.Credential, &cfg); err != nil {
		problem(w, http.StatusBadRequest, "invalid provider credential")
		return
	}
	if _, err := qrisprovider.NewOpenAPI(cfg); err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	cipher, err := s.Cipher.Encrypt(in.Credential)
	if err != nil {
		problem(w, 500, "encrypt credential failed")
		return
	}
	v := domain.MerchantAccount{ID: in.ID, TenantID: in.TenantID, Provider: in.Provider, Name: in.Name, CredentialCiphertext: cipher, Active: true, CreatedAt: time.Now().UTC()}
	if err = s.Repo.CreateMerchantAccount(r.Context(), v); err != nil {
		problem(w, 500, "create merchant account failed")
		return
	}
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: v.ID + "-created", TenantID: v.TenantID, Actor: "admin", Action: "merchant_account.created", ResourceType: "merchant_account", ResourceID: v.ID, CreatedAt: time.Now().UTC()})
	v.CredentialCiphertext = ""
	write(w, 201, v)
}
func (s Server) testMerchant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	tenant := r.URL.Query().Get("tenant_id")
	m, err := s.Repo.MerchantAccount(r.Context(), tenant, r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	p, err := s.Gateway.Provider(r.Context(), m)
	if err == nil {
		err = p.Health(r.Context())
	}
	if err != nil {
		problem(w, 502, "provider connection failed")
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}

type merchantAccountView struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	Provider   string    `json:"provider"`
	Name       string    `json:"name"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	MerchantID string    `json:"merchant_id"`
	BaseURL    string    `json:"base_url"`
	CreatePath string    `json:"create_path,omitempty"`
	CheckPath  string    `json:"check_path,omitempty"`
}

func (s Server) merchantAccountView(m domain.MerchantAccount) (merchantAccountView, error) {
	view := merchantAccountView{ID: m.ID, TenantID: m.TenantID, Provider: m.Provider, Name: m.Name, Active: m.Active, CreatedAt: m.CreatedAt}
	plain, err := s.Cipher.Decrypt(m.CredentialCiphertext)
	if err != nil {
		return view, err
	}
	var cfg qrisprovider.OpenAPIConfig
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return view, err
	}
	view.MerchantID, view.BaseURL, view.CreatePath, view.CheckPath = cfg.MerchantID, cfg.BaseURL, cfg.CreatePath, cfg.CheckPath
	return view, nil
}

func (s Server) getMerchant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	m, err := s.Repo.MerchantAccount(r.Context(), r.URL.Query().Get("tenant_id"), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	view, err := s.merchantAccountView(m)
	if err != nil {
		problem(w, http.StatusInternalServerError, "decrypt merchant credential failed")
		return
	}
	write(w, http.StatusOK, view)
}

func (s Server) updateMerchant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	current, err := s.Repo.MerchantAccount(r.Context(), r.URL.Query().Get("tenant_id"), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	var in struct {
		TenantID   string          `json:"tenant_id"`
		Name       string          `json:"name"`
		Active     bool            `json:"active"`
		Credential json.RawMessage `json:"credential"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.TenantID = strings.TrimSpace(in.TenantID)
	if in.Name == "" || in.TenantID == "" {
		problem(w, http.StatusBadRequest, "name and tenant_id are required")
		return
	}
	if target, tenantErr := s.Repo.Tenant(r.Context(), in.TenantID); tenantErr != nil || !target.Active {
		problem(w, http.StatusBadRequest, "active Tenant ID is required")
		return
	}
	current.Name, current.TenantID, current.Active = in.Name, in.TenantID, in.Active
	if len(in.Credential) > 0 {
		var cfg qrisprovider.OpenAPIConfig
		if err := json.Unmarshal(in.Credential, &cfg); err != nil {
			problem(w, http.StatusBadRequest, "invalid provider credential")
			return
		}
		if _, err := qrisprovider.NewOpenAPI(cfg); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		ciphertext, err := s.Cipher.Encrypt(in.Credential)
		if err != nil {
			problem(w, http.StatusInternalServerError, "encrypt credential failed")
			return
		}
		current.CredentialCiphertext = ciphertext
	}
	if err := s.Repo.UpdateMerchantAccount(r.Context(), current); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, err)
			return
		}
		problem(w, http.StatusInternalServerError, "update merchant account failed")
		return
	}
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), TenantID: current.TenantID, Actor: "admin", Action: "merchant_account.updated", ResourceType: "merchant_account", ResourceID: current.ID, CreatedAt: time.Now().UTC()})
	view, err := s.merchantAccountView(current)
	if err != nil {
		problem(w, http.StatusInternalServerError, "decrypt merchant credential failed")
		return
	}
	write(w, http.StatusOK, view)
}
func (s Server) listInvoices(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	limit, ok := listLimit(w, r)
	if !ok {
		return
	}
	v, err := s.Repo.ListInvoices(r.Context(), r.URL.Query().Get("tenant_id"), limit)
	respond(w, v, err)
}
func (s Server) getAdminInvoice(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.Invoice(r.Context(), r.URL.Query().Get("tenant_id"), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	write(w, http.StatusOK, v)
}
func (s Server) dashboard(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	items, err := s.Repo.ListInvoices(r.Context(), r.URL.Query().Get("tenant_id"), 100)
	if err != nil {
		problem(w, http.StatusInternalServerError, "repository error")
		return
	}
	var paid, pending int
	var volume int64
	for _, inv := range items {
		switch inv.Status {
		case domain.InvoicePaid:
			paid++
			volume += inv.Amount
		case domain.InvoicePending, domain.InvoiceCreating:
			pending++
		}
	}
	terminal := 0
	for _, inv := range items {
		if inv.Status != domain.InvoicePending && inv.Status != domain.InvoiceCreating {
			terminal++
		}
	}
	rate := 0.0
	if terminal > 0 {
		rate = float64(paid) / float64(terminal) * 100
	}
	write(w, http.StatusOK, map[string]any{
		"total_volume":     volume,
		"paid_invoices":    paid,
		"pending_invoices": pending,
		"success_rate":     rate,
		"recent_invoices":  items,
	})
}
func (s Server) adminHealth(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	type result struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Kind            string `json:"kind"`
		Status          string `json:"status"`
		LatencyMS       int64  `json:"latency_ms"`
		Endpoint        string `json:"endpoint,omitempty"`
		LastCheckedAt   string `json:"last_checked_at"`
		LastConnectedAt string `json:"last_connected_at,omitempty"`
		LastSyncedAt    string `json:"last_synced_at,omitempty"`
		UpdatedAt       string `json:"updated_at,omitempty"`
		Message         string `json:"message"`
	}
	now := time.Now().UTC()
	checkedAt := now.Format(time.RFC3339)
	out := []result{{ID: "backend-api", Name: "Go API", Kind: "backend_api", Status: "operational", Endpoint: "/v1 + /admin", LastCheckedAt: checkedAt, LastConnectedAt: checkedAt, Message: "Backend API menerima pemeriksaan kesehatan"}}
	database := result{ID: "database", Name: "PostgreSQL", Kind: "database", Status: "operational", Endpoint: "DATABASE_URL", LastCheckedAt: checkedAt, Message: "Koneksi database aktif"}
	started := time.Now()
	if err := s.Repo.Health(r.Context()); err != nil {
		database.Status, database.Message = "offline", "Koneksi database gagal"
	} else {
		database.LastConnectedAt = checkedAt
	}
	database.LatencyMS = time.Since(started).Milliseconds()
	out = append(out, database)

	adminData := result{ID: "admin-data", Name: "Admin data", Kind: "database", Status: "operational", Endpoint: "/admin/tenants + /admin/qris-templates", LastCheckedAt: checkedAt, Message: "Tabel data admin siap dibaca"}
	started = time.Now()
	if _, err := s.Repo.ListTenants(r.Context()); err != nil {
		adminData.Status, adminData.Message = "offline", "Tabel tenant tidak dapat dibaca"
	} else if _, err := s.Repo.ListQRISTemplates(r.Context()); err != nil {
		adminData.Status, adminData.Message = "offline", "Tabel QRIS template tidak dapat dibaca"
	} else {
		adminData.LastConnectedAt = checkedAt
	}
	adminData.LatencyMS = time.Since(started).Milliseconds()
	out = append(out, adminData)

	merchantIDs, err := s.Repo.ListMerchantIDs(r.Context())
	if err != nil {
		out = append(out, result{ID: "browser-sessions", Name: "Browser sessions", Kind: "browser_session", Status: "offline", Endpoint: "merchant.qris.interactive.co.id", LastCheckedAt: checkedAt, Message: "Schema browser session belum siap"})
	} else {
		for _, merchantID := range merchantIDs {
			item := result{ID: "browser-" + merchantID.ID, Name: merchantID.Name, Kind: "browser_session", Status: "offline", Endpoint: "merchant.qris.interactive.co.id", LastCheckedAt: checkedAt, Message: "Session browser belum tersambung"}
			connection, connectionErr := s.Repo.MerchantConnection(r.Context(), merchantID.ID)
			if connectionErr == nil {
				item.UpdatedAt = connection.UpdatedAt.Format(time.RFC3339)
				if connection.LastSyncedAt != nil {
					item.LastConnectedAt = connection.LastSyncedAt.Format(time.RFC3339)
					item.LastSyncedAt = item.LastConnectedAt
				}
				switch connection.Status {
				case domain.ConnectionConnected:
					item.Status, item.Message = "operational", "Session browser tersambung dan dikelola worker"
				case domain.ConnectionReconnectRequired, domain.ConnectionExpired:
					item.Status, item.Message = "degraded", "Session browser memerlukan pemulihan koneksi"
				default:
					item.Status, item.Message = "offline", "Session browser tidak tersambung"
				}
				if connection.LastError != "" {
					item.Message = connection.LastError
				}
			} else if !errors.Is(connectionErr, store.ErrNotFound) {
				item.Message = "Status session browser tidak dapat dibaca"
			}
			out = append(out, item)
		}
	}

	merchants, err := s.Repo.ListMerchantAccounts(r.Context(), r.URL.Query().Get("tenant_id"))
	if err != nil {
		problem(w, http.StatusInternalServerError, "repository error")
		return
	}
	for _, merchant := range merchants {
		// This is an optional direct-provider probe. Browser reconciliation uses
		// the separate browser_session item above and remains independent.
		item := result{ID: "provider-" + merchant.ID, Name: merchant.Name + " - Direct API", Kind: "provider_api", Status: "operational", Endpoint: "InterActive QRIS API (opsional)", LastCheckedAt: checkedAt, Message: "Jalur direct API opsional dapat dijangkau"}
		started = time.Now()
		p, resolveErr := s.Gateway.Provider(r.Context(), merchant)
		if resolveErr == nil {
			resolveErr = p.Health(r.Context())
		}
		item.LatencyMS = time.Since(started).Milliseconds()
		if resolveErr != nil {
			item.Status, item.Message = "offline", "Jalur direct API opsional tidak tersedia; worker tetap memakai sesi browser"
		} else {
			item.LastConnectedAt = checkedAt
		}
		out = append(out, item)
	}
	write(w, http.StatusOK, out)
}
func (s Server) listAudit(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	limit, ok := listLimit(w, r)
	if !ok {
		return
	}
	v, err := s.Repo.ListAudit(r.Context(), r.URL.Query().Get("tenant_id"), limit)
	respond(w, v, err)
}
func (s Server) listQRSTemplates(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.ListQRISTemplates(r.Context())
	for i := range v {
		v[i].StaticPayload = ""
	}
	respond(w, v, err)
}
func (s Server) createQRSTemplate(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if err := r.ParseMultipartForm(6 << 20); err != nil {
		problem(w, http.StatusBadRequest, "multipart upload is required")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		problem(w, http.StatusBadRequest, "image file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(http.MaxBytesReader(w, file, 5<<20))
	if err != nil {
		problem(w, http.StatusBadRequest, "could not read image")
		return
	}
	mime := http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" {
		problem(w, http.StatusBadRequest, "only PNG and JPEG QR images are supported")
		return
	}
	template, err := qrisservice.DecodeImage(data, mime)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	template.ID, template.Name, template.ImageData = newID(), strings.TrimSpace(r.FormValue("name")), data
	template.TenantID = strings.TrimSpace(r.FormValue("tenant_id"))
	template.AccessScope = strings.TrimSpace(r.FormValue("access_scope"))
	if template.AccessScope == "" {
		template.AccessScope = "all_tenants"
	}
	template.StaticToDynamic = r.FormValue("static_to_dynamic") == "true" || r.FormValue("static_to_dynamic") == "on"
	template.MaxRequestsPM, _ = strconv.Atoi(r.FormValue("max_requests_per_minute"))
	if template.MaxRequestsPM == 0 {
		template.MaxRequestsPM = 60
	}
	template.Active = true
	if template.AccessScope != "all_tenants" && template.AccessScope != "selected_tenant" {
		problem(w, http.StatusBadRequest, "invalid QRIS access scope")
		return
	}
	if template.MaxRequestsPM < 1 || template.MaxRequestsPM > 10000 {
		problem(w, http.StatusBadRequest, "max requests per minute must be between 1 and 10000")
		return
	}
	if template.AccessScope == "all_tenants" {
		template.TenantID = ""
	} else if template.TenantID != "" {
		tenant, tenantErr := s.Repo.Tenant(r.Context(), template.TenantID)
		if tenantErr != nil || !tenant.Active {
			problem(w, http.StatusBadRequest, "active Tenant ID is required")
			return
		}
	} else {
		problem(w, http.StatusBadRequest, "Tenant ID is required for selected tenant access")
		return
	}
	if template.Name == "" {
		template.Name = header.Filename
	}
	template.CreatedAt = time.Now().UTC()
	if err := s.Repo.CreateQRISTemplate(r.Context(), template); err != nil {
		problem(w, http.StatusInternalServerError, "save QRIS template failed")
		return
	}
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), TenantID: template.TenantID, Actor: "admin", Action: "qris_template.created", ResourceType: "qris_template", ResourceID: template.ID, Metadata: map[string]any{"static_to_dynamic": template.StaticToDynamic}, CreatedAt: template.CreatedAt})
	template.StaticPayload = ""
	template.ImageData = nil
	write(w, http.StatusCreated, template)
}
func (s Server) updateQRSTemplate(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	template, err := s.Repo.QRISTemplate(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	var in struct {
		Name            string `json:"name"`
		TenantID        string `json:"tenant_id"`
		AccessScope     string `json:"access_scope"`
		StaticToDynamic bool   `json:"static_to_dynamic"`
		MaxRequestsPM   int    `json:"max_requests_per_minute"`
		Active          bool   `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name, in.TenantID = strings.TrimSpace(in.Name), strings.TrimSpace(in.TenantID)
	if in.Name == "" || (in.AccessScope != "all_tenants" && in.AccessScope != "selected_tenant") || in.MaxRequestsPM < 1 || in.MaxRequestsPM > 10000 {
		problem(w, http.StatusBadRequest, "name, valid access scope, and max requests between 1 and 10000 are required")
		return
	}
	if in.AccessScope == "all_tenants" {
		in.TenantID = ""
	} else {
		tenant, tenantErr := s.Repo.Tenant(r.Context(), in.TenantID)
		if tenantErr != nil || !tenant.Active {
			problem(w, http.StatusBadRequest, "active Tenant ID is required")
			return
		}
	}
	template.Name, template.TenantID, template.AccessScope = in.Name, in.TenantID, in.AccessScope
	template.StaticToDynamic, template.MaxRequestsPM, template.Active = in.StaticToDynamic, in.MaxRequestsPM, in.Active
	if err = s.Repo.UpdateQRISTemplate(r.Context(), template); err != nil {
		problem(w, http.StatusInternalServerError, "update QRIS template failed")
		return
	}
	template.StaticPayload, template.ImageData = "", nil
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), Actor: "admin", Action: "qris_template.updated", ResourceType: "qris_template", ResourceID: template.ID, Metadata: map[string]any{"access_scope": template.AccessScope, "max_requests_per_minute": template.MaxRequestsPM}, CreatedAt: time.Now().UTC()})
	write(w, http.StatusOK, template)
}
func (s Server) qrisTemplateImage(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	template, err := s.Repo.QRISTemplate(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	w.Header().Set("Content-Type", template.ImageMIME)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(template.ImageData)
}
func (s Server) createTestPayment(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var in struct {
		QRISTemplateID string `json:"qris_template_id"`
		Amount         int64  `json:"amount"`
	}
	if !decode(w, r, &in) {
		return
	}
	template, err := s.Repo.QRISTemplate(r.Context(), in.QRISTemplateID)
	if err != nil {
		notFound(w, err)
		return
	}
	payload, err := qrisservice.Convert(template.StaticPayload, in.Amount)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	uniqueCode := newUniqueCode()
	payload, err = qrisservice.WithBillNumber(payload, uniqueCode)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	tenantID, merchantID := template.TenantID, ""
	if tenantID != "" {
		tenant, tenantErr := s.Repo.Tenant(r.Context(), tenantID)
		if tenantErr == nil {
			merchantID = tenant.MerchantID
		}
	}
	if merchantID == "" {
		merchants, listErr := s.Repo.ListMerchantIDs(r.Context())
		if listErr != nil {
			problem(w, http.StatusInternalServerError, "merchant lookup failed")
			return
		}
		for _, merchant := range merchants {
			if !merchant.Active {
				continue
			}
			if merchantID != "" {
				problem(w, http.StatusConflict, "shared QRIS template must be assigned to a tenant when multiple Merchant IDs are active")
				return
			}
			merchantID = merchant.ID
		}
	}
	if merchantID == "" {
		problem(w, http.StatusConflict, "QRIS template is not linked to an active Merchant ID")
		return
	}
	matchConfidence := "waiting_first_check"
	payment := domain.TestPayment{
		ID: newID(), QRISTemplateID: template.ID, MerchantID: merchantID, TenantID: tenantID,
		Amount: in.Amount, PayableAmount: in.Amount, DynamicPayload: payload, UniqueCode: uniqueCode, Status: domain.InvoicePending,
		RequestSource: "admin_qris_test", MatchConfidence: matchConfidence,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}
	nextCheck := now.Add(15 * time.Second)
	payment.NextCheckAt = &nextCheck
	if err := s.Repo.CreateTestPayment(r.Context(), payment); err != nil {
		if errors.Is(err, store.ErrUniqueAmountUnavailable) {
			problem(w, http.StatusConflict, "another pending transaction uses the same merchant and amount")
			return
		}
		problem(w, http.StatusInternalServerError, "save test payment failed")
		return
	}
	write(w, http.StatusCreated, payment)
}
func (s Server) listTestPayments(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	if _, err := s.Repo.ExpirePendingTestPayments(r.Context(), time.Now().UTC()); err != nil {
		problem(w, http.StatusInternalServerError, "expire test payments failed")
		return
	}
	v, err := s.Repo.ListTestPayments(r.Context(), 100)
	respond(w, v, err)
}
func (s Server) testPaymentQR(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	payment, err := s.Repo.TestPayment(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, store.ErrNotFound)
		return
	}
	png, err := qrisservice.PNG(payment.DynamicPayload)
	if err != nil {
		problem(w, http.StatusInternalServerError, "QR generation failed")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}
func (s Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Repo.Health(r.Context()); err != nil {
		problem(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	write(w, http.StatusOK, map[string]string{"status": "ok"})
}
func listLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	const defaultLimit = 100
	const maxLimit = 500
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxLimit {
		problem(w, http.StatusBadRequest, "limit must be between 1 and 500")
		return 0, false
	}
	return limit, true
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		problem(w, 400, "invalid JSON")
		return false
	}
	return true
}
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		problem(w, 500, "repository error")
		return
	}
	write(w, 200, v)
}
func notFound(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		problem(w, 404, "not found")
		return
	}
	problem(w, 502, "operation failed")
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]string{"error": msg})
}

func newID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}

func (s Server) enqueueBrowserJob(ctx context.Context, kind, merchantID string, priority int) (domain.BrowserJob, bool, error) {
	now := time.Now().UTC()
	return s.Repo.EnqueueBrowserJob(ctx, domain.BrowserJob{ID: newID(), ResourceKey: "neko-shared", MerchantID: merchantID, Kind: kind, Priority: priority, State: "queued", NotBefore: now, RequestedAt: now, RequestCount: 1})
}

func secureUniqueAmountCodeOrder() ([]int64, error) {
	codes := make([]int64, 99)
	for i := range codes {
		codes[i] = int64(i + 1)
	}
	for i := len(codes) - 1; i > 0; i-- {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return nil, err
		}
		j := int(index.Int64())
		codes[i], codes[j] = codes[j], codes[i]
	}
	return codes, nil
}

func newUniqueCode() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	n := uint64(b[0])<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
	return strconv.FormatUint(10_000_000+n%90_000_000, 10)
}

func newAPIKey() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return "xl_live_" + base64.RawURLEncoding.EncodeToString(bytes[:])
}

func validIntegrationURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	if len(raw) > 2048 {
		return false
	}
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http")
}

func validUniqueAmountCooldown(minutes int) bool {
	return minutes >= 30 && minutes <= 60
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
