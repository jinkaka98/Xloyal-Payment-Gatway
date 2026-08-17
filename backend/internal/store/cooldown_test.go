package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"xloyal/backend/internal/domain"
)

func TestPaidUniqueAmountStaysReservedUntilTenantCooldownEnds(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", UniqueAmountCooldownMinutes: 45, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-a"})

	first := domain.TestPayment{ID: "payment-a", IdempotencyKey: "order-a", QRISTemplateID: "template-a", MerchantID: "merchant-a", TenantID: "tenant-a", Amount: 1000, PayableAmount: 1037, UniqueAmountCode: 37, Status: domain.InvoicePending, RequestSource: "tenant_api", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	if _, _, _, _, err := repo.CreateTenantTestPayment(ctx, first, now, 100); err != nil {
		t.Fatal(err)
	}
	paidAt := now.Add(time.Minute)
	first.Status = domain.InvoicePaid
	first.CheckCount = 1
	first.UpdatedAt = paidAt
	first.LastCheckedAt = &paidAt
	first.NextCheckAt = nil
	first.MatchedTransactionID = "portal-a"
	if updated, err := repo.MatchPendingTestPayment(ctx, first, domain.PortalTransaction{ID: "portal-a", MerchantID: "merchant-a", Reference: "ref-a", Amount: 1037, Status: "paid", PaidAt: paidAt, CreatedAt: paidAt}); err != nil || !updated {
		t.Fatalf("paid transition updated=%v err=%v", updated, err)
	}

	reuse := first
	reuse.ID, reuse.IdempotencyKey, reuse.Status, reuse.MatchedTransactionID = "payment-b", "order-b", domain.InvoicePending, ""
	reuse.CheckCount, reuse.LastCheckedAt = 0, nil
	reuse.CreatedAt, reuse.UpdatedAt, reuse.ExpiresAt = paidAt.Add(44*time.Minute), paidAt.Add(44*time.Minute), paidAt.Add(74*time.Minute)
	if _, _, _, _, err := repo.CreateTenantTestPayment(ctx, reuse, reuse.CreatedAt, 100); !errors.Is(err, ErrUniqueAmountUnavailable) {
		t.Fatalf("reuse during cooldown err=%v", err)
	}

	reuse.CreatedAt, reuse.UpdatedAt, reuse.ExpiresAt = paidAt.Add(45*time.Minute), paidAt.Add(45*time.Minute), paidAt.Add(75*time.Minute)
	if _, created, _, _, err := repo.CreateTenantTestPayment(ctx, reuse, reuse.CreatedAt, 100); err != nil || !created {
		t.Fatalf("reuse after cooldown created=%v err=%v", created, err)
	}

	audits, err := repo.ListAudit(ctx, "tenant-a", 20)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, event := range audits {
		actions[event.Action] = true
	}
	for _, action := range []string{"qris.unique_amount.reserved", "qris.unique_amount.cooldown_started", "qris.unique_amount.cooldown_ended"} {
		if !actions[action] {
			t.Fatalf("missing audit action %q: %+v", action, audits)
		}
	}
}

func TestExpiredUniqueAmountUsesCooldownBeforeReuse(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	now := time.Date(2026, 8, 17, 13, 0, 0, 0, time.UTC)
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", UniqueAmountCooldownMinutes: 30, Active: true})
	repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-a"})
	first := domain.TestPayment{ID: "payment-expired", IdempotencyKey: "expired-order", QRISTemplateID: "template-a", MerchantID: "merchant-a", TenantID: "tenant-a", Amount: 1000, PayableAmount: 1012, UniqueAmountCode: 12, Status: domain.InvoicePending, RequestSource: "tenant_api", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if _, _, _, _, err := repo.CreateTenantTestPayment(ctx, first, now, 100); err != nil {
		t.Fatal(err)
	}
	if count, err := repo.ExpirePendingTestPayments(ctx, first.ExpiresAt); err != nil || count != 1 {
		t.Fatalf("expired count=%d err=%v", count, err)
	}

	reuse := first
	reuse.ID, reuse.IdempotencyKey, reuse.Status = "payment-next", "next-order", domain.InvoicePending
	reuse.CreatedAt, reuse.UpdatedAt, reuse.ExpiresAt = first.ExpiresAt.Add(29*time.Minute), first.ExpiresAt.Add(29*time.Minute), first.ExpiresAt.Add(59*time.Minute)
	if _, _, _, _, err := repo.CreateTenantTestPayment(ctx, reuse, reuse.CreatedAt, 100); !errors.Is(err, ErrUniqueAmountUnavailable) {
		t.Fatalf("expired code reused during cooldown err=%v", err)
	}
	reuse.CreatedAt, reuse.UpdatedAt, reuse.ExpiresAt = first.ExpiresAt.Add(30*time.Minute), first.ExpiresAt.Add(30*time.Minute), first.ExpiresAt.Add(60*time.Minute)
	if _, created, _, _, err := repo.CreateTenantTestPayment(ctx, reuse, reuse.CreatedAt, 100); err != nil || !created {
		t.Fatalf("expired code after cooldown created=%v err=%v", created, err)
	}
}
