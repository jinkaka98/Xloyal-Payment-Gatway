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
