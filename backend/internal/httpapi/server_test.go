package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/security"
	"xloyal/backend/internal/store"
)

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
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), `"status":"pending"`) || !strings.Contains(create.Body.String(), "540550000") {
		t.Fatalf("payment code=%d body=%s", create.Code, create.Body.String())
	}
}
