package worker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/store"
)

func TestValidationCycleProcessesOneHundredFiftyClientsWithOneBrowserSync(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant-test", Status: domain.ConnectionConnected, UpdatedAt: now.Add(-time.Hour)})
	for i := 0; i < 150; i++ {
		createdAt := now.Add(-time.Minute).Add(time.Duration(i) * time.Millisecond)
		if err := repo.CreateTestPayment(ctx, domain.TestPayment{
			ID: "payment-" + strconv.Itoa(i), QRISTemplateID: "template-test", MerchantID: "merchant-test", TenantID: "tenant-" + strconv.Itoa(i),
			Amount: int64(10_000 + i), PayableAmount: int64(10_000 + i), Status: domain.InvoicePending, RequestSource: "tenant_api",
			CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: now.Add(10 * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	syncCalls := 0
	w := Worker{
		Repo: repo, Now: func() time.Time { return now }, Logger: slog.New(slog.NewJSONHandler(&output, nil)),
		SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) {
			syncCalls++
			return nil, nil
		},
	}
	if err := w.validateTestPayments(ctx); err != nil {
		t.Fatal(err)
	}
	if syncCalls != 1 {
		t.Fatalf("browser sync calls=%d", syncCalls)
	}
	for i := 0; i < 150; i++ {
		payment, err := repo.TestPayment(ctx, "payment-"+strconv.Itoa(i))
		if err != nil || payment.CheckCount != 1 {
			t.Fatalf("payment %d check_count=%d err=%v", i, payment.CheckCount, err)
		}
	}
	logOutput := output.String()
	if !strings.Contains(logOutput, `"msg":"qris validation cycle completed"`) || !strings.Contains(logOutput, `"queued_payments":150`) || !strings.Contains(logOutput, `"merchants":1`) {
		t.Fatalf("cycle log=%s", logOutput)
	}
}

func TestValidationCycleSkipsDisconnectedPaymentsBeforeHealthyMerchant(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 17, 14, 30, 0, 0, time.UTC)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant-healthy", Status: domain.ConnectionConnected, UpdatedAt: now.Add(-time.Hour)})
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant-disconnected", Status: domain.ConnectionReconnectRequired, UpdatedAt: now.Add(-time.Hour)})
	for i := 0; i < TestPaymentBatchSize; i++ {
		createdAt := now.Add(-2 * time.Minute).Add(time.Duration(i) * time.Millisecond)
		if err := repo.CreateTestPayment(ctx, domain.TestPayment{ID: "disconnected-" + strconv.Itoa(i), QRISTemplateID: "template-test", MerchantID: "merchant-disconnected", Amount: int64(20_000 + i), PayableAmount: int64(20_000 + i), Status: domain.InvoicePending, CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: now.Add(time.Minute), NextCheckAt: ptrTime(now.Add(-time.Second))}); err != nil {
			t.Fatal(err)
		}
	}
	createdAt := now.Add(-time.Minute)
	if err := repo.CreateTestPayment(ctx, domain.TestPayment{ID: "healthy-payment", QRISTemplateID: "template-test", MerchantID: "merchant-healthy", Amount: 10_000, PayableAmount: 10_000, Status: domain.InvoicePending, CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: now.Add(time.Minute), NextCheckAt: ptrTime(now.Add(-time.Second))}); err != nil {
		t.Fatal(err)
	}
	w := Worker{Repo: repo, Now: func() time.Time { return now }, SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) { return nil, nil }}
	if err := w.validateTestPayments(ctx); err != nil {
		t.Fatal(err)
	}
	healthy, err := repo.TestPayment(ctx, "healthy-payment")
	if err != nil || healthy.CheckCount != 1 {
		t.Fatalf("healthy payment was starved check_count=%d err=%v", healthy.CheckCount, err)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestManualLoginBrowserJobQueuesFollowupSyncThroughWorker(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant-a", Status: domain.ConnectionReconnectRequired, UpdatedAt: now})
	_, _, err := repo.EnqueueBrowserJob(ctx, domain.BrowserJob{ID: "manual-job", ResourceKey: "neko-shared", MerchantID: "merchant-a", Kind: "manual_login", Priority: 100, State: "queued", NotBefore: now, RequestedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	manualCalls, syncCalls := 0, 0
	w := Worker{
		Repo: repo, JobOwner: "worker-test", Now: func() time.Time { return now },
		ManualLogin: func(context.Context, domain.MerchantConnection) error {
			manualCalls++
			return nil
		},
		SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) {
			syncCalls++
			return nil, nil
		},
	}
	processed, err := w.processNextBrowserJob(ctx)
	if err != nil || !processed || manualCalls != 1 || syncCalls != 0 {
		t.Fatalf("manual processed=%v manual=%d sync=%d err=%v", processed, manualCalls, syncCalls, err)
	}
	processed, err = w.processNextBrowserJob(ctx)
	if err != nil || !processed || syncCalls != 1 {
		t.Fatalf("followup processed=%v sync=%d err=%v", processed, syncCalls, err)
	}
	connection, _ := repo.MerchantConnection(ctx, "merchant-a")
	if connection.Status != domain.ConnectionConnected || connection.LastError != "" {
		t.Fatalf("connection=%+v", connection)
	}
}

func TestOnePortalTransactionCannotArbitrarilyPaySameAmountRequests(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Minute)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	for _, id := range []string{"payment-a", "payment-b"} {
		repo.CreateTestPayment(ctx, domain.TestPayment{
			ID: id, QRISTemplateID: "template-test", MerchantID: "merchant-test", TenantID: "tenant-test",
			Amount: 1013, Status: domain.InvoicePending, RequestSource: "legacy_fixture",
			CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: now.Add(10 * time.Minute),
		})
	}
	repo.CreatePortalTransaction(ctx, domain.PortalTransaction{
		ID: "portal-one", MerchantID: "merchant-test", Reference: "portal-ref", Amount: 1013,
		Status: "paid", PaidAt: createdAt.Add(20 * time.Second), Source: "browser", CreatedAt: now,
	})

	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	if err := w.checkTestPayments(ctx, now); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"payment-a", "payment-b"} {
		payment, _ := repo.TestPayment(ctx, id)
		if payment.Status != domain.InvoicePending {
			t.Fatalf("ambiguous request %s was marked %s", id, payment.Status)
		}
	}
}

func TestUniquePayableAmountMatchesTheCorrectSameValueOrder(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Minute)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.CreateTestPayment(ctx, domain.TestPayment{ID: "payment-a", QRISTemplateID: "template-test", MerchantID: "merchant-test", TenantID: "tenant-a", Amount: 10000, PayableAmount: 10001, UniqueAmountCode: 1, Status: domain.InvoicePending, RequestSource: "tenant_api", CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: now.Add(10 * time.Minute)})
	repo.CreateTestPayment(ctx, domain.TestPayment{ID: "payment-b", QRISTemplateID: "template-test", MerchantID: "merchant-test", TenantID: "tenant-b", Amount: 10000, PayableAmount: 10002, UniqueAmountCode: 2, Status: domain.InvoicePending, RequestSource: "tenant_api", CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: now.Add(10 * time.Minute)})
	repo.CreatePortalTransaction(ctx, domain.PortalTransaction{ID: "portal-b", MerchantID: "merchant-test", Reference: "portal-ref", Amount: 10002, Status: "paid", PaidAt: createdAt.Add(20 * time.Second), Source: "browser", CreatedAt: now})

	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	if err := w.checkTestPayments(ctx, now); err != nil {
		t.Fatal(err)
	}
	a, _ := repo.TestPayment(ctx, "payment-a")
	b, _ := repo.TestPayment(ctx, "payment-b")
	if a.Status != domain.InvoicePending || b.Status != domain.InvoicePaid || b.MatchedTransactionID != "portal-b" {
		t.Fatalf("payment-a=%+v payment-b=%+v", a, b)
	}
}

type failingPortalRepository struct {
	store.Repository
	err error
}

type microsecondConnectionRepository struct{ store.Repository }

func (r microsecondConnectionRepository) UpsertMerchantConnection(ctx context.Context, connection domain.MerchantConnection) error {
	connection.UpdatedAt = connection.UpdatedAt.Truncate(time.Microsecond)
	if connection.LastSyncedAt != nil {
		truncated := connection.LastSyncedAt.Truncate(time.Microsecond)
		connection.LastSyncedAt = &truncated
	}
	return r.Repository.UpsertMerchantConnection(ctx, connection)
}

func TestQRISTestValidationAcceptsPostgresTimestampPrecision(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	repo := microsecondConnectionRepository{Repository: memory}
	now := time.Unix(1_786_677_600, 123_456_789).UTC()
	createdAt := now.Add(-time.Minute)
	memory.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	memory.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant-test", Status: domain.ConnectionConnected, UpdatedAt: now.Add(-time.Minute)})
	memory.CreateTestPayment(ctx, domain.TestPayment{ID: "precision-test", QRISTemplateID: "template-test", MerchantID: "merchant-test", Amount: 1013, Status: domain.InvoicePending, CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: now.Add(time.Minute)})
	w := Worker{Repo: repo, Now: func() time.Time { return now }, SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) { return nil, nil }}
	if err := w.validateTestPayments(ctx); err != nil {
		t.Fatal(err)
	}
	payment, _ := memory.TestPayment(ctx, "precision-test")
	if payment.CheckCount != 1 || payment.LastCheckedAt == nil {
		t.Fatalf("successful sync was rejected after timestamp truncation: %+v", payment)
	}
}

func (r failingPortalRepository) CreatePortalTransaction(context.Context, domain.PortalTransaction) error {
	return r.err
}

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

func TestQRISTestPaymentExpiresWithoutFabricatingCheck(t *testing.T) {
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
	if payment.Status != domain.InvoiceExpired || payment.MatchConfidence != "expired_no_match" || payment.CheckCount != 0 || payment.LastCheckedAt != nil {
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

func TestQRISTestPaymentDoesNotMatchTransactionBeforeRequest(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 5, 0, 0, time.UTC)
	createdAt := now.Add(-5 * time.Minute)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID: "test-pending", QRISTemplateID: "template-test", MerchantID: "merchant-test",
		Amount: 50000, Status: domain.InvoicePending, RequestSource: "admin_qris_test",
		CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: createdAt.Add(30 * time.Minute),
	})
	repo.CreatePortalTransaction(ctx, domain.PortalTransaction{
		ID: "portal-before-request", MerchantID: "merchant-test", Reference: "portal-ref", Amount: 50000,
		Status: "paid", PaidAt: createdAt.Add(-time.Minute), Source: "browser", CreatedAt: now,
	})
	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	if err := w.checkTestPayments(ctx, now); err != nil {
		t.Fatal(err)
	}
	payment, _ := repo.TestPayment(ctx, "test-pending")
	if payment.Status != domain.InvoicePending || payment.MatchedTransactionID != "" || payment.CheckCount != 1 {
		t.Fatalf("transaction before request was matched: %+v", payment)
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

func TestQRISTestValidationSyncsBrowserBeforeCountingCheck(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	startedAt := time.Date(2026, 8, 14, 1, 28, 0, 0, time.UTC)
	completedAt := startedAt.Add(45 * time.Second)
	current := startedAt
	createdAt := startedAt.Add(-30 * time.Second)
	recentSync := startedAt.Add(-time.Minute)

	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{
		MerchantID:   "merchant-test",
		Status:       domain.ConnectionConnected,
		LastSyncedAt: &recentSync,
		UpdatedAt:    recentSync,
	})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID:             "test-fresh-sync",
		QRISTemplateID: "template-test",
		MerchantID:     "merchant-test",
		Amount:         1013,
		Status:         domain.InvoicePending,
		RequestSource:  "admin_qris_test",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		ExpiresAt:      createdAt.Add(30 * time.Minute),
	})

	w := Worker{
		Repo: repo,
		Now:  func() time.Time { return current },
		SyncMerchant: func(ctx context.Context, connection domain.MerchantConnection) ([]domain.PortalTransaction, error) {
			payment, err := repo.TestPayment(ctx, "test-fresh-sync")
			if err != nil {
				t.Fatal(err)
			}
			if payment.CheckCount != 0 || payment.LastCheckedAt != nil {
				t.Fatalf("payment counted before browser history completed: %+v", payment)
			}
			current = completedAt
			return []domain.PortalTransaction{{
				Reference: "portal-reference",
				Amount:    1013,
				Status:    "paid",
				PaidAt:    createdAt.Add(21 * time.Second),
			}}, nil
		},
	}

	if err := w.validateTestPayments(ctx); err != nil {
		t.Fatal(err)
	}
	payment, _ := repo.TestPayment(ctx, "test-fresh-sync")
	if payment.Status != domain.InvoicePaid || payment.CheckCount != 1 || payment.MatchedTransactionID == "" {
		t.Fatalf("payment was not matched from the completed browser refresh: %+v", payment)
	}
	if payment.LastCheckedAt == nil || !payment.LastCheckedAt.Equal(completedAt) {
		t.Fatalf("completed check timestamp=%v want=%v", payment.LastCheckedAt, completedAt)
	}
}

func TestQRISTestValidationDoesNotCountFailedBrowserSync(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 14, 1, 28, 0, 0, time.UTC)
	createdAt := now.Add(-30 * time.Second)

	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{
		MerchantID: "merchant-test",
		Status:     domain.ConnectionConnected,
		UpdatedAt:  now,
	})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID:             "test-failed-sync",
		QRISTemplateID: "template-test",
		MerchantID:     "merchant-test",
		Amount:         1013,
		Status:         domain.InvoicePending,
		RequestSource:  "admin_qris_test",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		ExpiresAt:      createdAt.Add(30 * time.Minute),
	})
	w := Worker{
		Repo: repo,
		Now:  func() time.Time { return now },
		SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) {
			return nil, errors.New("portal history request failed")
		},
	}

	if err := w.validateTestPayments(ctx); err != nil {
		t.Fatal(err)
	}
	payment, _ := repo.TestPayment(ctx, "test-failed-sync")
	if payment.Status != domain.InvoicePending || payment.CheckCount != 0 || payment.LastCheckedAt != nil {
		t.Fatalf("failed browser request was counted as a check: %+v", payment)
	}
	connection, _ := repo.MerchantConnection(ctx, "merchant-test")
	if connection.Status != domain.ConnectionReconnectRequired || connection.LastError == "" {
		t.Fatalf("browser failure diagnostic was not preserved: %+v", connection)
	}
}

func TestQRISTestValidationDoesNotCountFailedLedgerPersistence(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	repo := failingPortalRepository{Repository: memory, err: errors.New("database write failed")}
	now := time.Date(2026, 8, 14, 1, 28, 0, 0, time.UTC)
	createdAt := now.Add(-30 * time.Second)

	memory.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	memory.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant-test", Status: domain.ConnectionConnected, UpdatedAt: now})
	memory.CreateTestPayment(ctx, domain.TestPayment{
		ID: "test-ledger-failure", QRISTemplateID: "template-test", MerchantID: "merchant-test",
		Amount: 1013, Status: domain.InvoicePending, RequestSource: "admin_qris_test",
		CreatedAt: createdAt, UpdatedAt: createdAt, ExpiresAt: createdAt.Add(30 * time.Minute),
	})
	w := Worker{
		Repo: repo,
		Now:  func() time.Time { return now },
		SyncMerchant: func(context.Context, domain.MerchantConnection) ([]domain.PortalTransaction, error) {
			return []domain.PortalTransaction{{Reference: "portal-ref", Amount: 1013, Status: "paid", PaidAt: now}}, nil
		},
	}

	if err := w.validateTestPayments(ctx); err != nil {
		t.Fatal(err)
	}
	payment, _ := memory.TestPayment(ctx, "test-ledger-failure")
	if payment.Status != domain.InvoicePending || payment.CheckCount != 0 || payment.LastCheckedAt != nil {
		t.Fatalf("failed ledger write was counted as a check: %+v", payment)
	}
	connection, _ := memory.MerchantConnection(ctx, "merchant-test")
	if connection.Status != domain.ConnectionReconnectRequired || connection.LastError == "" {
		t.Fatalf("ledger failure was marked as a successful sync: %+v", connection)
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

func TestMerchantSyncKeepsConnectedStatusWhileCheckerRuns(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	repo.UpsertMerchantConnection(ctx, domain.MerchantConnection{MerchantID: "merchant_1", Status: domain.ConnectionConnected, UpdatedAt: now.Add(-10 * time.Minute)})
	w := Worker{Repo: repo, Now: func() time.Time { return now }, SyncMerchant: func(ctx context.Context, connection domain.MerchantConnection) ([]domain.PortalTransaction, error) {
		current, err := repo.MerchantConnection(ctx, connection.MerchantID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status != domain.ConnectionConnected || current.LastError != "Browser sync in progress" {
			t.Fatalf("healthy connection was presented as disconnected during sync: %+v", current)
		}
		return nil, context.DeadlineExceeded
	}}
	if err := w.syncMerchants(ctx, now); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileInvoiceMatchesByUniqueReference(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", MerchantID: "merchant-a", Active: true})
	repo.CreateInvoice(ctx, domain.Invoice{ID: "invoice-a", TenantID: "tenant-a", IdempotencyKey: "idem", Amount: 75000, ProviderReference: "INV-123", Status: domain.InvoicePending, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)})
	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	transaction := domain.PortalTransaction{MerchantID: "merchant-a", Reference: "INV-123", Amount: 75000, Status: "paid", PaidAt: now.Add(-time.Hour)}
	w.reconcileTransaction(ctx, &transaction, now)
	if transaction.MatchConfidence != "reference_unique" || transaction.InvoiceID != "invoice-a" || transaction.TenantID != "tenant-a" {
		t.Fatalf("transaction=%+v", transaction)
	}
	invoice, _ := repo.Invoice(ctx, "tenant-a", "invoice-a")
	if invoice.Status != domain.InvoicePaid {
		t.Fatalf("invoice=%+v", invoice)
	}
}

func TestQRISTestPaymentMatchesByUniqueReference(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	now := time.Date(2026, 8, 11, 12, 5, 0, 0, time.UTC)
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-test"})
	repo.CreateTestPayment(ctx, domain.TestPayment{
		ID: "test-paid", QRISTemplateID: "template-test", MerchantID: "merchant-test", TenantID: "tenant-test",
		Amount: 50000, UniqueCode: "12345678", Status: domain.InvoicePending, RequestSource: "admin_qris_test",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
	})
	repo.CreatePortalTransaction(ctx, domain.PortalTransaction{
		ID: "portal-test", MerchantID: "merchant-test", Reference: "trx-12345678-end", Amount: 50000,
		Status: "paid", PaidAt: now.Add(-time.Hour), Source: "browser", CreatedAt: now,
	})
	w := Worker{Repo: repo, Now: func() time.Time { return now }}
	if err := w.checkTestPayments(ctx, now); err != nil {
		t.Fatal(err)
	}
	payment, _ := repo.TestPayment(ctx, "test-paid")
	if payment.Status != domain.InvoicePaid || payment.MatchConfidence != "reference_unique" || payment.MatchedTransactionID != "portal-test" {
		t.Fatalf("payment=%+v", payment)
	}
	transactions, _ := repo.ListPortalTransactions(ctx, "merchant-test", "tenant-test", 10)
	if len(transactions) != 1 || transactions[0].MatchConfidence != "qris_test_reference_unique" {
		t.Fatalf("transactions=%+v", transactions)
	}
}
