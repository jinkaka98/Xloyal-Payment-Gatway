package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

func TestCreatePaymentSessionFoundation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	repo := store.NewMemory()
	_ = repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", Active: true})
	invoiceExpiry := now.Add(30 * time.Minute)
	_, _, _ = repo.CreateInvoice(ctx, domain.Invoice{ID: "invoice-a", TenantID: "tenant-a", IdempotencyKey: "idem-a", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now, ExpiresAt: invoiceExpiry})
	repo.PutAllowedRedirectURL(domain.AllowedRedirectURL{ID: "redirect-1", TenantID: "tenant-a", Type: "SUCCESS", URL: "https://client.example/success", Active: true})
	repo.PutPaymentThemeVersion(domain.PaymentTheme{ID: "theme-a", TenantID: "tenant-a", Status: "PUBLISHED", IsDefault: true}, domain.PaymentThemeVersion{ID: "theme-a-v1", ThemeID: "theme-a", Version: 1, Status: "PUBLISHED", Config: []byte(`{}`)})
	repo.PutPaymentThemeVersion(domain.PaymentTheme{ID: "theme-a", TenantID: "tenant-a", Status: "PUBLISHED", IsDefault: true}, domain.PaymentThemeVersion{ID: "theme-a-v2", ThemeID: "theme-a", Version: 2, Status: "PUBLISHED", Config: []byte(`{"schema_version":1}`)})
	service := PaymentSessionService{Repo: repo, Now: func() time.Time { return now }}
	result, err := service.CreatePaymentSession(ctx, CreatePaymentSessionInput{TenantID: "tenant-a", InvoiceID: "invoice-a", ThemeID: "theme-a", ThemeVersion: 1, SuccessURL: "https://client.example/success", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if result.PublicToken == "" || result.Session.PublicTokenHash == result.PublicToken {
		t.Fatal("plaintext token must be returned once and never stored")
	}
	if !result.Session.ExpiresAt.Equal(invoiceExpiry) {
		t.Fatalf("session expiry must be bounded by invoice expiry")
	}
	lookedUp, err := service.LookupPaymentSessionByPublicToken(ctx, result.PublicToken)
	if err != nil || lookedUp.ID != result.Session.ID {
		t.Fatalf("token lookup failed: %#v %v", lookedUp, err)
	}
	if _, err := service.LookupPaymentSessionByToken(ctx, "wrong-token"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("wrong token must be rejected, got %v", err)
	}
	_, _, _ = repo.CreateInvoice(ctx, domain.Invoice{ID: "invoice-default", TenantID: "tenant-a", IdempotencyKey: "idem-default", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, ExpiresAt: invoiceExpiry})
	defaultResult, err := service.CreatePaymentSession(ctx, CreatePaymentSessionInput{TenantID: "tenant-a", InvoiceID: "invoice-default"})
	if err != nil || defaultResult.Session.ThemeID != "theme-a" || defaultResult.Session.ThemeVersion != 2 {
		t.Fatalf("default published theme was not resolved: %#v %v", defaultResult.Session, err)
	}
	_, _, _ = repo.CreateInvoice(ctx, domain.Invoice{ID: "invoice-latest", TenantID: "tenant-a", IdempotencyKey: "idem-latest", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, ExpiresAt: invoiceExpiry})
	latest, err := service.CreatePaymentSession(ctx, CreatePaymentSessionInput{TenantID: "tenant-a", InvoiceID: "invoice-latest", ThemeID: "theme-a"})
	if err != nil || latest.Session.ThemeVersion != 2 {
		t.Fatalf("latest published theme was not selected: %#v %v", latest.Session, err)
	}
}

func TestPaymentSessionCancelIsIdempotentUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := store.NewMemory()
	repo.CreateInvoice(ctx, domain.Invoice{ID: "cancel-invoice", TenantID: "tenant", IdempotencyKey: "cancel", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)})
	service := PaymentSessionService{Repo: repo, Now: func() time.Time { return now }}
	created, err := service.CreatePaymentSession(ctx, CreatePaymentSessionInput{TenantID: "tenant", InvoiceID: "cancel-invoice"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, cancelErr := service.Cancel(ctx, created.PublicToken)
			errs <- cancelErr
		}()
	}
	wg.Wait()
	close(errs)
	for cancelErr := range errs {
		if cancelErr != nil {
			t.Fatalf("concurrent cancel failed: %v", cancelErr)
		}
	}
	snapshot, err := service.Snapshot(ctx, created.PublicToken)
	if err != nil || snapshot.Session.Status != domain.PaymentSessionCancelled {
		t.Fatalf("final status=%+v err=%v", snapshot.Session, err)
	}
}

func TestCreatePaymentSessionRejectsNonPendingInvoiceAndRedirect(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := store.NewMemory()
	_, _, _ = repo.CreateInvoice(ctx, domain.Invoice{ID: "paid", TenantID: "tenant-a", IdempotencyKey: "paid", Amount: 1000, Status: domain.InvoicePaid, ExpiresAt: now.Add(time.Hour)})
	service := PaymentSessionService{Repo: repo, Now: func() time.Time { return now }}
	if _, err := service.CreatePaymentSession(ctx, CreatePaymentSessionInput{TenantID: "tenant-a", InvoiceID: "paid"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	_, _, _ = repo.CreateInvoice(ctx, domain.Invoice{ID: "pending", TenantID: "tenant-a", IdempotencyKey: "pending", Amount: 1000, Status: domain.InvoicePending, ExpiresAt: now.Add(time.Hour)})
	if _, err := service.CreatePaymentSession(ctx, CreatePaymentSessionInput{TenantID: "tenant-a", InvoiceID: "pending", SuccessURL: "https://evil.example"}); err == nil {
		t.Fatal("unregistered redirect URL accepted")
	}
}

func TestPaymentSessionSnapshotUsesCurrentDefaultThemeForExistingURL(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	repo := store.NewMemory()
	_, _, _ = repo.CreateInvoice(ctx, domain.Invoice{ID: "live-theme-invoice", TenantID: "tenant-live", IdempotencyKey: "live-theme", Amount: 1000, Currency: "IDR", Status: domain.InvoicePending, CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)})
	repo.PutPaymentThemeVersion(domain.PaymentTheme{ID: "theme-old", TenantID: "tenant-live", Status: domain.ThemePublished, IsDefault: true}, domain.PaymentThemeVersion{ID: "theme-old-v1", ThemeID: "theme-old", Version: 1, Status: domain.ThemePublished, Config: []byte(`{"schema_version":1,"template_key":"modern"}`)})
	service := PaymentSessionService{Repo: repo, Now: func() time.Time { return now }}
	created, err := service.CreatePaymentSession(ctx, CreatePaymentSessionInput{TenantID: "tenant-live", InvoiceID: "live-theme-invoice"})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.Snapshot(ctx, created.PublicToken)
	if err != nil || initial.Theme == nil || initial.Theme.ThemeID != "theme-old" {
		t.Fatalf("initial theme=%+v err=%v", initial.Theme, err)
	}
	repo.PutPaymentThemeVersion(domain.PaymentTheme{ID: "theme-old", TenantID: "tenant-live", Status: domain.ThemePublished, IsDefault: false}, domain.PaymentThemeVersion{ID: "theme-old-v1", ThemeID: "theme-old", Version: 1, Status: domain.ThemePublished, Config: []byte(`{"schema_version":1,"template_key":"modern"}`)})
	repo.PutPaymentThemeVersion(domain.PaymentTheme{ID: "theme-new", TenantID: "tenant-live", Status: domain.ThemePublished, IsDefault: true}, domain.PaymentThemeVersion{ID: "theme-new-v1", ThemeID: "theme-new", Version: 1, Status: domain.ThemePublished, Config: []byte(`{"schema_version":1,"template_key":"dark"}`)})
	live, err := service.Snapshot(ctx, created.PublicToken)
	if err != nil || live.Theme == nil || live.Theme.ThemeID != "theme-new" {
		t.Fatalf("live theme=%+v err=%v", live.Theme, err)
	}
}
