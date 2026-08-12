package worker

import (
	"context"
	"testing"
	"time"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/store"
)

type pendingProvider struct{ checks int }

func (p *pendingProvider) CreatePayment(context.Context, domain.CreatePaymentRequest) (domain.CreatePaymentResult, error) {
	return domain.CreatePaymentResult{}, nil
}
func (p *pendingProvider) CheckPayment(context.Context, domain.CheckPaymentRequest) (domain.CheckPaymentResult, error) {
	p.checks++
	return domain.CheckPaymentResult{Status: domain.InvoicePending}, nil
}
func (*pendingProvider) Health(context.Context) error { return nil }

type failingProvider struct{}

func (failingProvider) CreatePayment(context.Context, domain.CreatePaymentRequest) (domain.CreatePaymentResult, error) {
	return domain.CreatePaymentResult{}, nil
}
func (failingProvider) CheckPayment(context.Context, domain.CheckPaymentRequest) (domain.CheckPaymentResult, error) {
	return domain.CheckPaymentResult{}, context.DeadlineExceeded
}
func (failingProvider) Health(context.Context) error { return nil }
func TestRetryAndExpiryPolicy(t *testing.T) {
	ctx := context.Background()
	r := store.NewMemory()
	r.CreateMerchantAccount(ctx, domain.MerchantAccount{ID: "m", TenantID: "t", Active: true})
	p := &pendingProvider{}
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	g := gateway.Service{Repo: r, Provider: func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error) { return p, nil }, Now: func() time.Time { return now }}
	fresh := domain.Invoice{ID: "fresh", TenantID: "t", MerchantAccountID: "m", IdempotencyKey: "1", Status: domain.InvoicePending, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	old := domain.Invoice{ID: "old", TenantID: "t", MerchantAccountID: "m", IdempotencyKey: "2", Status: domain.InvoicePending, CreatedAt: now.Add(-31 * time.Minute), ExpiresAt: now.Add(time.Minute)}
	r.CreateInvoice(ctx, fresh)
	r.CreateInvoice(ctx, old)
	w := Worker{Repo: r, Gateway: g, Now: func() time.Time { return now }}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	a, _ := r.Invoice(ctx, "t", "fresh")
	b, _ := r.Invoice(ctx, "t", "old")
	if a.CheckCount != 1 || p.checks != 1 || b.Status != domain.InvoiceExpired {
		t.Fatalf("fresh=%+v old=%+v checks=%d", a, b, p.checks)
	}
}

func TestFailedPollConsumesRetrySlot(t *testing.T) {
	ctx := context.Background()
	r := store.NewMemory()
	r.CreateMerchantAccount(ctx, domain.MerchantAccount{ID: "m", TenantID: "t", Active: true})
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	inv := domain.Invoice{ID: "retry", TenantID: "t", MerchantAccountID: "m", IdempotencyKey: "1", Status: domain.InvoicePending, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	r.CreateInvoice(ctx, inv)
	g := gateway.Service{Repo: r, Provider: func(context.Context, domain.MerchantAccount) (domain.PaymentProvider, error) {
		return failingProvider{}, nil
	}, Now: func() time.Time { return now }}
	if err := (Worker{Repo: r, Gateway: g, Now: func() time.Time { return now }}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Invoice(ctx, "t", "retry")
	if got.CheckCount != 1 || got.LastCheckedAt == nil {
		t.Fatalf("retry metadata not persisted: %+v", got)
	}
}

func TestQRISTestPaymentExpiresAndRecordsCheck(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 30, 0, 0, time.UTC)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID: "test-expired", QRISTemplateID: "template-test", MerchantID: "merchant-test",
		Amount: 25000, Status: domain.InvoicePending, RequestSource: "admin_qris_test",
		CreatedAt: now.Add(-31 * time.Minute), UpdatedAt: now.Add(-31 * time.Minute), ExpiresAt: now.Add(-time.Minute),
	})
	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	if err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	payment, _ := repo.TestPayment(ctx, "test-expired")
	if payment.Status != domain.InvoiceExpired || payment.MatchConfidence != "expired_no_match" || payment.CheckCount != 1 || payment.LastCheckedAt == nil {
		t.Fatalf("payment=%+v", payment)
	}
}

func TestQRISTestPaymentMatchesUniqueMerchantTransaction(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 5, 0, 0, time.UTC)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID: "test-paid", QRISTemplateID: "template-test", MerchantID: "merchant-test", TenantID: "tenant-test",
		Amount: 50000, Status: domain.InvoicePending, RequestSource: "admin_qris_test",
		CreatedAt: now.Add(-5 * time.Minute), UpdatedAt: now.Add(-5 * time.Minute), ExpiresAt: now.Add(25 * time.Minute),
	})
	repo.CreatePortalTransaction(ctx, domain.PortalTransaction{
		ID: "portal-test", MerchantID: "merchant-test", Reference: "portal-ref", Amount: 50000,
		Status: "paid", PaidAt: now.Add(-time.Minute), Source: "browser", CreatedAt: now,
	})
	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	if err := w.checkTestPayments(ctx, now); err != nil {
		t.Fatal(err)
	}
	payment, _ := repo.TestPayment(ctx, "test-paid")
	if payment.Status != domain.InvoicePaid || payment.MatchConfidence != "amount_time_unique" || payment.MatchedTransactionID != "portal-test" {
		t.Fatalf("payment=%+v", payment)
	}
	transactions, _ := repo.ListPortalTransactions(ctx, "merchant-test", "tenant-test", 10)
	if len(transactions) != 1 || transactions[0].MatchConfidence != "qris_test_amount_time_unique" {
		t.Fatalf("transactions=%+v", transactions)
	}
}

func TestQRISTestSmartCheckWaitsAndSchedulesNextAttempt(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-smart"})
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant-smart", Status: domain.ConnectionConnected, UpdatedAt: now})
	first := now.Add(30 * time.Second)
	repo.CreateTestPayment(ctx, domain.TestPayment{ID: "smart-1", QRISTemplateID: "template-smart", MerchantID: "merchant-smart", Amount: 1000, Status: domain.InvoicePending, MatchConfidence: "waiting_first_check", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(5 * time.Minute), NextCheckAt: &first})
	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	if err := w.checkTestPayments(ctx, now); err != nil {
		t.Fatal(err)
	}
	payment, _ := repo.TestPayment(ctx, "smart-1")
	if payment.CheckCount != 0 {
		t.Fatalf("payment checked before due time: %+v", payment)
	}
	if err := w.checkTestPayments(ctx, first); err != nil {
		t.Fatal(err)
	}
	payment, _ = repo.TestPayment(ctx, "smart-1")
	if payment.CheckCount != 1 || payment.LastCheckedAt == nil || payment.NextCheckAt == nil || !payment.NextCheckAt.Equal(first.Add(TestPaymentPollInterval)) {
		t.Fatalf("smart schedule not persisted: %+v", payment)
	}
}

func TestMerchantBrowserSyncPersistsPortalLedger(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	if err := repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant_1", Status: domain.ConnectionConnected}); err != nil {
		t.Fatal(err)
	}
	called := false
	w := Worker{
		Repo:    repo,
		Gateway: gateway.Service{},
		Now:     func() time.Time { return now },
		SyncMerchant: func(_ context.Context, connection domain.MerchantConnection) ([]domain.PortalTransaction, error) {
			called = connection.MerchantID == "merchant_1"
			return []domain.PortalTransaction{{Reference: "portal-ref-1", Amount: 25000, PaidAt: now, Status: "paid"}}, nil
		},
	}
	if err := w.syncMerchants(ctx, now); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListPortalTransactions(ctx, "merchant_1", "", 10)
	if err != nil || !called || len(items) != 1 || items[0].Source != "browser" || items[0].ID[:10] != "merchant_1" {
		t.Fatalf("called=%v items=%+v err=%v", called, items, err)
	}
	connection, _ := repo.MerchantConnection(ctx, "merchant_1")
	if connection.Status != domain.ConnectionConnected || connection.LastSyncedAt == nil {
		t.Fatalf("connection=%+v", connection)
	}
}

func TestMerchantRefreshMatchesPreviouslyUnassignedTransaction(t *testing.T) {
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	repo.CreateTenant(context.Background(), domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", Active: true})
	repo.UpsertMerchantConnection(context.Background(), domain.MerchantConnection{MerchantID: "merchant-a", Status: domain.ConnectionConnected, UpdatedAt: time.Unix(0, 0)})
	w := Worker{Repo: repo, Now: func() time.Time { return now }, SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) {
		return []domain.PortalTransaction{{Reference: "portal-ref", Amount: 75000, Status: "paid", PaidAt: now}}, nil
	}}
	if err := w.syncMerchants(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	first, _ := repo.ListPortalTransactions(context.Background(), "merchant-a", "", 10)
	if len(first) != 1 || first[0].TenantID != "" {
		t.Fatalf("first sync=%+v", first)
	}
	repo.CreateInvoice(context.Background(), domain.Invoice{ID: "invoice-a", TenantID: "tenant-a", IdempotencyKey: "idem", Amount: 75000, Status: domain.InvoicePending, CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(30 * time.Minute)})
	connection, _ := repo.MerchantConnection(context.Background(), "merchant-a")
	connection.UpdatedAt, connection.LastSyncedAt = time.Unix(0, 0), nil
	repo.UpsertMerchantConnection(context.Background(), connection)
	if err := w.syncMerchants(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	matched, _ := repo.ListPortalTransactions(context.Background(), "merchant-a", "tenant-a", 10)
	if len(matched) != 1 || matched[0].InvoiceID != "invoice-a" || matched[0].MatchConfidence != "amount_time_unique" {
		t.Fatalf("matched sync=%+v", matched)
	}
	invoice, _ := repo.Invoice(context.Background(), "tenant-a", "invoice-a")
	if invoice.Status != domain.InvoicePaid {
		t.Fatalf("invoice status=%s", invoice.Status)
	}
}

func TestReconnectRequiredConnectionIsProcessed(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant_1", Status: domain.ConnectionReconnectRequired, UpdatedAt: now.Add(-10 * time.Minute)})
	called := false
	w := Worker{Repo: repo, Gateway: gateway.Service{}, Now: func() time.Time { return now }, SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) {
		called = true
		return nil, nil
	}}
	if err := w.syncMerchants(ctx, now); err != nil {
		t.Fatal(err)
	}
	connection, _ := repo.MerchantConnection(ctx, "merchant_1")
	if !called || connection.Status != domain.ConnectionConnected {
		t.Fatalf("called=%v connection=%+v", called, connection)
	}
}
