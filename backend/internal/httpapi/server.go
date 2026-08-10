package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
}

func (s Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /v1/health", s.health)
	m.HandleFunc("POST /v1/tenants/{tenant_id}/invoices", s.public(s.createInvoice))
	m.HandleFunc("GET /v1/invoices/{invoice_id}", s.public(s.getInvoice))
	m.HandleFunc("POST /v1/invoices/{invoice_id}/check", s.public(s.checkInvoice))
	m.HandleFunc("GET /v1/invoices/{invoice_id}/qr", s.public(s.qr))
	m.HandleFunc("GET /admin/tenants", s.admin("viewer", s.listTenants))
	m.HandleFunc("POST /admin/tenants", s.admin("super_admin", s.createTenant))
	m.HandleFunc("GET /admin/merchant-accounts", s.admin("viewer", s.listMerchants))
	m.HandleFunc("POST /admin/merchant-accounts", s.admin("operator", s.createMerchant))
	m.HandleFunc("POST /admin/merchant-accounts/{id}/test-connection", s.admin("operator", s.testMerchant))
	m.HandleFunc("GET /admin/invoices", s.admin("viewer", s.listInvoices))
	m.HandleFunc("GET /admin/invoices/{id}", s.admin("viewer", s.getAdminInvoice))
	m.HandleFunc("GET /admin/dashboard", s.admin("viewer", s.dashboard))
	m.HandleFunc("GET /admin/health", s.admin("viewer", s.adminHealth))
	m.HandleFunc("GET /admin/qris-templates", s.admin("viewer", s.listQRSTemplates))
	m.HandleFunc("POST /admin/qris-templates", s.admin("operator", s.createQRSTemplate))
	m.HandleFunc("GET /admin/qris-templates/{id}/image", s.admin("viewer", s.qrisTemplateImage))
	m.HandleFunc("GET /admin/qris-test-payments", s.admin("viewer", s.listTestPayments))
	m.HandleFunc("POST /admin/qris-test-payments", s.admin("operator", s.createTestPayment))
	m.HandleFunc("GET /admin/qris-test-payments/{id}/qr", s.admin("viewer", s.testPaymentQR))
	m.HandleFunc("GET /admin/audit-events", s.admin("viewer", s.listAudit))
	return securityHeaders(m)
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
		ID     string `json:"id"`
		Name   string `json:"name"`
		APIKey string `json:"api_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.ID == "" || in.Name == "" || len(in.APIKey) < 24 {
		problem(w, 400, "id, name and API key of at least 24 characters are required")
		return
	}
	v := domain.Tenant{ID: in.ID, Name: in.Name, APIKeyHash: security.HashSecret(in.APIKey), Active: true, CreatedAt: time.Now().UTC()}
	if err := s.Repo.CreateTenant(r.Context(), v); err != nil {
		problem(w, 500, "create tenant failed")
		return
	}
	v.APIKeyHash = ""
	write(w, 201, v)
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
	if in.ID == "" || in.TenantID == "" || in.Name == "" || in.Provider != "openapi" {
		problem(w, http.StatusBadRequest, "id, tenant_id, name and provider=openapi are required")
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
		ID            string `json:"id"`
		Name          string `json:"name"`
		Status        string `json:"status"`
		LatencyMS     int64  `json:"latency_ms"`
		LastCheckedAt string `json:"last_checked_at"`
		Message       string `json:"message"`
	}
	now := time.Now().UTC()
	out := []result{{ID: "database", Name: "PostgreSQL", Status: "operational", LastCheckedAt: now.Format(time.RFC3339), Message: "Database connection is healthy"}}
	if err := s.Repo.Health(r.Context()); err != nil {
		out[0].Status, out[0].Message = "offline", "Database connection failed"
	}
	merchants, err := s.Repo.ListMerchantAccounts(r.Context(), r.URL.Query().Get("tenant_id"))
	if err != nil {
		problem(w, http.StatusInternalServerError, "repository error")
		return
	}
	for _, merchant := range merchants {
		item := result{ID: merchant.ID, Name: merchant.Name, Status: "operational", LastCheckedAt: now.Format(time.RFC3339), Message: "Provider connection is healthy"}
		started := time.Now()
		p, resolveErr := s.Gateway.Provider(r.Context(), merchant)
		if resolveErr == nil {
			resolveErr = p.Health(r.Context())
		}
		item.LatencyMS = time.Since(started).Milliseconds()
		if resolveErr != nil {
			item.Status, item.Message = "offline", "Provider connection failed"
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
	if template.Name == "" {
		template.Name = header.Filename
	}
	template.CreatedAt = time.Now().UTC()
	if err := s.Repo.CreateQRISTemplate(r.Context(), template); err != nil {
		problem(w, http.StatusInternalServerError, "save QRIS template failed")
		return
	}
	template.StaticPayload = ""
	template.ImageData = nil
	write(w, http.StatusCreated, template)
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
	payment := domain.TestPayment{ID: newID(), QRISTemplateID: template.ID, Amount: in.Amount, DynamicPayload: payload, Status: domain.InvoicePending, CreatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	if err := s.Repo.CreateTestPayment(r.Context(), payment); err != nil {
		problem(w, http.StatusInternalServerError, "save test payment failed")
		return
	}
	write(w, http.StatusCreated, payment)
}
func (s Server) listTestPayments(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	v, err := s.Repo.ListTestPayments(r.Context(), 100)
	respond(w, v, err)
}
func (s Server) testPaymentQR(w http.ResponseWriter, r *http.Request, _ domain.Tenant) {
	items, err := s.Repo.ListTestPayments(r.Context(), 500)
	if err != nil {
		problem(w, http.StatusInternalServerError, "repository error")
		return
	}
	var payment domain.TestPayment
	for _, item := range items {
		if item.ID == r.PathValue("id") {
			payment = item
			break
		}
	}
	if payment.ID == "" {
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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
