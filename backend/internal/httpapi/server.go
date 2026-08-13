package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
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
	Repo        store.Repository
	Gateway     gateway.Service
	Cipher      *security.Cipher
	AdminTokens map[string]string
	ManualLogin func(context.Context, domain.MerchantConnection) error
}

func (s Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /v1/health", s.health)
	m.HandleFunc("POST /v1/tenants/{tenant_id}/invoices", s.public(s.createInvoice))
	m.HandleFunc("POST /v1/tenants/{tenant_id}/transactions/refresh", s.public(s.refreshTenantTransactions))
	m.HandleFunc("GET /v1/tenants/{tenant_id}/transactions", s.public(s.listTenantTransactions))
	m.HandleFunc("POST /v1/tenants/{tenant_id}/qris/dynamic", s.public(s.createTenantDynamicQRIS))
	m.HandleFunc("GET /v1/invoices/{invoice_id}", s.public(s.getInvoice))
	m.HandleFunc("POST /v1/invoices/{invoice_id}/check", s.public(s.checkInvoice))
	m.HandleFunc("GET /v1/invoices/{invoice_id}/qr", s.public(s.qr))
	m.HandleFunc("GET /admin/tenants", s.admin("viewer", s.listTenants))
	m.HandleFunc("POST /admin/tenants", s.admin("super_admin", s.createTenant))
	m.HandleFunc("PUT /admin/tenants/{id}", s.admin("super_admin", s.updateTenant))
	m.HandleFunc("GET /admin/merchant-ids", s.admin("viewer", s.listMerchantIDs))
	m.HandleFunc("POST /admin/merchant-ids", s.admin("operator", s.createMerchantID))
	m.HandleFunc("GET /admin/merchant-ids/{id}/connection", s.admin("viewer", s.getMerchantConnection))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/session", s.admin("operator", s.importMerchantSession))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/har", s.admin("operator", s.importMerchantHAR))
	m.HandleFunc("POST /admin/merchant-connections/har", s.admin("operator", s.importDefaultMerchantHAR))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/revoke", s.admin("operator", s.revokeMerchantSession))
	m.HandleFunc("POST /admin/merchant-ids/{id}/sync", s.admin("operator", s.requestMerchantSync))
	m.HandleFunc("POST /admin/merchant-ids/{id}/connection/manual-login", s.admin("operator", s.startManualMerchantLogin))
	m.HandleFunc("GET /admin/merchant-ids/{id}/tariff", s.admin("viewer", s.getTariff))
	m.HandleFunc("PUT /admin/merchant-ids/{id}/tariff", s.admin("operator", s.putTariff))
	m.HandleFunc("GET /admin/merchant-transactions", s.admin("viewer", s.listMerchantTransactions))
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
	return securityHeaders(m)
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
	connection.Status = domain.ConnectionReconnectRequired
	connection.LastError = "Manual browser login in progress"
	connection.LastSyncedAt = nil
	connection.UpdatedAt = time.Now().UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), connection); err != nil {
		problem(w, http.StatusInternalServerError, "mark manual browser login failed")
		return
	}
	go s.finishManualLogin(connection)
	write(w, http.StatusAccepted, map[string]string{"status": "manual_login_started"})
}

func (s Server) finishManualLogin(connection domain.MerchantConnection) {
	ctx := context.Background()
	merchantID := connection.MerchantID
	if err := s.ManualLogin(ctx, connection); err != nil {
		failed, lookupErr := s.Repo.MerchantConnection(ctx, merchantID)
		if lookupErr != nil {
			return
		}
		failed.Status = domain.ConnectionReconnectRequired
		failed.LastError = "Manual browser login failed: " + truncateManualLoginError(err.Error())
		failed.UpdatedAt = time.Now().UTC()
		_ = s.Repo.UpsertMerchantConnection(ctx, failed)
		return
	}
	queued, err := s.Repo.MerchantConnection(ctx, merchantID)
	if err != nil {
		return
	}
	queued.Status = domain.ConnectionReconnectRequired
	queued.LastSyncedAt = nil
	queued.LastError = "Browser connection queued"
	queued.UpdatedAt = time.Unix(0, 0).UTC()
	_ = s.Repo.UpsertMerchantConnection(ctx, queued)
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
	// Epoch makes the existing scheduler pick this connection immediately.
	v.UpdatedAt = time.Unix(0, 0).UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), v); err != nil {
		problem(w, 500, "queue sync failed")
		return
	}
	write(w, 202, map[string]string{"status": "queued"})
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
	connection.UpdatedAt = time.Unix(0, 0).UTC()
	if err = s.Repo.UpsertMerchantConnection(r.Context(), connection); err != nil {
		problem(w, http.StatusInternalServerError, "queue refresh failed")
		return
	}
	write(w, http.StatusAccepted, map[string]any{
		"status": "queued", "tenant_id": tenant.ID, "merchant_id": tenant.MerchantID,
		"poll_url": "/v1/tenants/" + tenant.ID + "/transactions",
	})
}
func (s Server) listTenantTransactions(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	limit, ok := listLimit(w, r)
	if !ok {
		return
	}
	transactions, err := s.Repo.ListPortalTransactions(r.Context(), tenant.MerchantID, tenant.ID, limit)
	respond(w, transactions, err)
}
func (s Server) createTenantDynamicQRIS(w http.ResponseWriter, r *http.Request, tenant domain.Tenant) {
	var in struct {
		TemplateID string `json:"template_id"`
		Amount     int64  `json:"amount"`
	}
	if !decode(w, r, &in) {
		return
	}
	template, err := s.Repo.QRISTemplate(r.Context(), in.TemplateID)
	accessScope := template.AccessScope
	if accessScope == "" && template.TenantID != "" {
		accessScope = "selected_tenant"
	}
	if accessScope == "" {
		accessScope = "all_tenants"
	}
	if err != nil || (accessScope == "selected_tenant" && template.TenantID != tenant.ID) {
		notFound(w, store.ErrNotFound)
		return
	}
	if !template.Active || !template.StaticToDynamic {
		problem(w, http.StatusConflict, "QRIS template is not enabled for dynamic tenant requests")
		return
	}
	payload, err := qrisservice.Convert(template.StaticPayload, in.Amount)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	maxRequests := template.MaxRequestsPM
	if maxRequests < 1 {
		maxRequests = 60
	}
	allowed, retryAfter, err := s.Repo.AllowQRISRequest(r.Context(), template.ID, tenant.ID, now, maxRequests)
	if err != nil {
		problem(w, http.StatusInternalServerError, "QRIS rate limit check failed")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		problem(w, http.StatusTooManyRequests, "QRIS request limit exceeded")
		return
	}
	png, err := qrisservice.PNG(payload)
	if err != nil {
		problem(w, http.StatusInternalServerError, "QRIS image generation failed")
		return
	}
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), TenantID: tenant.ID, Actor: "tenant_api", Action: "qris.dynamic_generated", ResourceType: "qris_template", ResourceID: template.ID, Metadata: map[string]any{"amount": in.Amount}, CreatedAt: now})
	write(w, http.StatusCreated, map[string]any{"tenant_id": tenant.ID, "template_id": template.ID, "amount": in.Amount, "currency": "IDR", "qr_payload": payload, "qr_png_base64": base64.StdEncoding.EncodeToString(png), "created_at": now})
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
		next(w, r, tenant)
	}
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
	inv, created, err := s.Gateway.CreateInvoice(r.Context(), gateway.CreateInvoiceInput{TenantID: t.ID, MerchantAccountID: in.MerchantAccountID, IdempotencyKey: in.IdempotencyKey, Amount: in.Amount, Currency: in.Currency, Description: in.Description})
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
	}
	respond(w, v, err)
}
func (s Server) createTenant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	var in struct {
		Name        string `json:"name"`
		MerchantID  string `json:"merchant_id"`
		SiteURL     string `json:"site_url"`
		CallbackURL string `json:"callback_url"`
		WebhookURL  string `json:"webhook_url"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.MerchantID == "" {
		problem(w, http.StatusBadRequest, "name and Merchant ID are required")
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
	v := domain.Tenant{ID: "tenant_" + newID()[:16], MerchantID: merchant.ID, Name: in.Name, SiteURL: strings.TrimSpace(in.SiteURL), CallbackURL: strings.TrimSpace(in.CallbackURL), WebhookURL: strings.TrimSpace(in.WebhookURL), APIKeyHash: security.HashSecret(apiKey), Active: true, CreatedAt: time.Now().UTC()}
	if err := s.Repo.CreateTenant(r.Context(), v); err != nil {
		problem(w, 500, "create tenant failed")
		return
	}
	v.APIKeyHash = ""
	write(w, 201, map[string]any{"tenant": v, "api_key": apiKey, "api_key_visible_once": true})
}
func (s Server) updateTenant(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	current, err := s.Repo.Tenant(r.Context(), r.PathValue("id"))
	if err != nil {
		notFound(w, err)
		return
	}
	var in struct {
		Name        string `json:"name"`
		MerchantID  string `json:"merchant_id"`
		SiteURL     string `json:"site_url"`
		CallbackURL string `json:"callback_url"`
		WebhookURL  string `json:"webhook_url"`
		Active      bool   `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.MerchantID == "" {
		problem(w, http.StatusBadRequest, "name and Merchant ID are required")
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
	current.Name, current.MerchantID, current.SiteURL, current.CallbackURL, current.WebhookURL, current.Active = in.Name, merchant.ID, strings.TrimSpace(in.SiteURL), strings.TrimSpace(in.CallbackURL), strings.TrimSpace(in.WebhookURL), in.Active
	if err = s.Repo.UpdateTenant(r.Context(), current); err != nil {
		problem(w, http.StatusInternalServerError, "update tenant failed")
		return
	}
	current.APIKeyHash = ""
	s.Repo.AppendAudit(r.Context(), domain.AuditEvent{ID: newID(), Actor: "admin", Action: "tenant.updated", ResourceType: "tenant", ResourceID: current.ID, TenantID: current.ID, CreatedAt: time.Now().UTC()})
	write(w, http.StatusOK, current)
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
		item := result{ID: "provider-" + merchant.ID, Name: merchant.Name, Kind: "provider_api", Status: "operational", Endpoint: "InterActive QRIS API", LastCheckedAt: checkedAt, Message: "API provider dapat dijangkau"}
		started = time.Now()
		p, resolveErr := s.Gateway.Provider(r.Context(), merchant)
		if resolveErr == nil {
			resolveErr = p.Health(r.Context())
		}
		item.LatencyMS = time.Since(started).Milliseconds()
		if resolveErr != nil {
			item.Status, item.Message = "offline", "Koneksi API provider gagal"
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
		Amount: in.Amount, DynamicPayload: payload, Status: domain.InvoicePending,
		RequestSource: "admin_qris_test", MatchConfidence: matchConfidence,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}
	nextCheck := now.Add(30 * time.Second)
	payment.NextCheckAt = &nextCheck
	if err := s.Repo.CreateTestPayment(r.Context(), payment); err != nil {
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

func truncateManualLoginError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 240 {
		return message
	}
	return message[:240]
}

func newID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
