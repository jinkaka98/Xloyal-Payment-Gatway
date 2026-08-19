package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"xloyal/backend/internal/domain"
)

func TestCancelAndWorkerPaidRaceHasOneTerminalWinner(t *testing.T) {
	newPayment := func(id string, now time.Time) domain.TestPayment {
		return domain.TestPayment{ID: id, QRISTemplateID: "template-a", MerchantID: "merchant-a", TenantID: "tenant-a", Amount: 1000, PayableAmount: 1000, Status: domain.InvoicePending, RequestSource: "tenant_api", MatchConfidence: "waiting_first_check", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	}
	newAudit := func(id string, now time.Time) domain.AuditEvent {
		return domain.AuditEvent{ID: "audit-" + id, TenantID: "tenant-a", Actor: "tenant_api", Action: "qris.transaction_cancelled", ResourceType: "test_payment", ResourceID: id, CreatedAt: now}
	}

	t.Run("cancel commits before stale worker", func(t *testing.T) {
		ctx := context.Background()
		repo := NewMemory()
		now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
		repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-a"})
		payment := newPayment("cancel-wins", now)
		if err := repo.CreateTestPayment(ctx, payment); err != nil {
			t.Fatal(err)
		}
		if cancelled, changed, err := repo.CancelPendingTestPayment(ctx, "tenant-a", payment.ID, now.Add(time.Second), newAudit(payment.ID, now.Add(time.Second))); err != nil || !changed || cancelled.Status != domain.InvoiceCancelled {
			t.Fatalf("cancelled=%+v changed=%v err=%v", cancelled, changed, err)
		}
		stale := payment
		stale.Status = domain.InvoicePaid
		stale.CheckCount = 1
		stale.UpdatedAt = now.Add(2 * time.Second)
		stale.NextCheckAt = nil
		if updated, err := repo.UpdatePendingTestPayment(ctx, stale); err != nil || updated {
			t.Fatalf("stale worker updated=%v err=%v", updated, err)
		}
		final, _ := repo.TestPayment(ctx, payment.ID)
		if final.Status != domain.InvoiceCancelled {
			t.Fatalf("final status=%s", final.Status)
		}
	})

	t.Run("worker commits paid before cancel", func(t *testing.T) {
		ctx := context.Background()
		repo := NewMemory()
		now := time.Date(2026, 8, 17, 15, 5, 0, 0, time.UTC)
		repo.CreateQRISTemplate(ctx, domain.QRISTemplate{ID: "template-a"})
		payment := newPayment("worker-wins", now)
		if err := repo.CreateTestPayment(ctx, payment); err != nil {
			t.Fatal(err)
		}
		paid := payment
		paid.Status = domain.InvoicePaid
		paid.CheckCount = 1
		paid.UpdatedAt = now.Add(time.Second)
		paid.NextCheckAt = nil
		if updated, err := repo.UpdatePendingTestPayment(ctx, paid); err != nil || !updated {
			t.Fatalf("worker updated=%v err=%v", updated, err)
		}
		current, changed, err := repo.CancelPendingTestPayment(ctx, "tenant-a", payment.ID, now.Add(2*time.Second), newAudit(payment.ID, now.Add(2*time.Second)))
		if !errors.Is(err, ErrConflict) || changed || current.Status != domain.InvoicePaid {
			t.Fatalf("current=%+v changed=%v err=%v", current, changed, err)
		}
	})
}
