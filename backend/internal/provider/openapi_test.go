package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xloyal/backend/internal/domain"
)

func TestOpenAPIContracts(t *testing.T) {
	var createOK, checkOK bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected authorization header")
		}
		if r.URL.Path == "/create" {
			q := r.URL.Query()
			createOK = r.Method == http.MethodGet && q.Get("do") == "create-invoice" && q.Get("apikey") == "fixture-token" && q.Get("mID") == "merchant-fixture" && q.Get("cliTrxNumber") == "inv-1" && q.Get("cliTrxAmount") == "1000" && q.Get("useTip") == "no"
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]string{"qris_content": "000201fixture", "qris_request_date": "2026-01-01", "qris_invoiceid": "ref-1", "qris_nmid": "NMIDFIXTURE"}})
			return
		}
		q := r.URL.Query()
		checkOK = r.Method == http.MethodGet && r.URL.Path == "/check" && q.Get("do") == "checkStatus" && q.Get("apikey") == "fixture-token" && q.Get("mID") == "merchant-fixture" && q.Get("invid") == "ref-1" && q.Get("trxvalue") == "1000" && q.Get("trxdate") == "2026-01-01"
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]string{"qris_status": "paid"}})
	}))
	defer s.Close()
	p, err := NewOpenAPI(OpenAPIConfig{BaseURL: s.URL, CreatePath: "/create", CheckPath: "/check", MerchantID: "merchant-fixture", APIKey: "fixture-token", Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	created, err := p.CreatePayment(context.Background(), domain.CreatePaymentRequest{InvoiceID: "inv-1", Amount: 1000})
	if err != nil || created.ProviderReference != "ref-1" || created.ProviderRequestDate != "2026-01-01" {
		t.Fatal(created, err)
	}
	checked, err := p.CheckPayment(context.Background(), domain.CheckPaymentRequest{ProviderInvoiceID: "ref-1", Amount: 1000, RequestDate: "2026-01-01"})
	if err != nil || checked.Status != domain.InvoicePaid || !createOK || !checkOK {
		t.Fatal(checked, err, createOK, checkOK)
	}
}
func TestOpenAPIMapsUnpaidToPending(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]string{"qris_status": "unpaid"}})
	}))
	defer s.Close()
	p, err := NewOpenAPI(OpenAPIConfig{BaseURL: s.URL, CheckPath: "/check", MerchantID: "merchant-fixture", APIKey: "fixture-token", Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.CheckPayment(context.Background(), domain.CheckPaymentRequest{})
	if err != nil || got.Status != domain.InvoicePending {
		t.Fatalf("status=%q err=%v", got.Status, err)
	}
}

func TestOpenAPIRejectsInsecureProductionURLAndInvalidQR(t *testing.T) {
	if _, err := NewOpenAPI(OpenAPIConfig{
		BaseURL: "http://provider.example", MerchantID: "merchant", APIKey: "token",
	}); err == nil {
		t.Fatal("insecure provider URL accepted")
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]string{
			"qris_content": "not-qris", "qris_request_date": "2026-01-01", "qris_invoiceid": "ref-1",
		}})
	}))
	defer s.Close()
	p, err := NewOpenAPI(OpenAPIConfig{
		BaseURL: s.URL, MerchantID: "merchant", APIKey: "token", Client: s.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.CreatePayment(context.Background(), domain.CreatePaymentRequest{InvoiceID: "invoice", Amount: 1000}); err == nil {
		t.Fatal("invalid QR payload accepted")
	}
}

func TestOpenAPIReportsUnknownProviderStatus(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]string{"qris_status": "review"}})
	}))
	defer s.Close()
	p, err := NewOpenAPI(OpenAPIConfig{
		BaseURL: s.URL, CheckPath: "/check", MerchantID: "merchant", APIKey: "token", Client: s.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.CheckPayment(context.Background(), domain.CheckPaymentRequest{})
	if err == nil || !strings.Contains(err.Error(), "review") {
		t.Fatalf("unexpected error=%v", err)
	}
}
