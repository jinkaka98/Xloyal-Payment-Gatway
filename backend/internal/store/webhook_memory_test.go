package store

import (
	"context"
	"testing"
	"time"

	"xloyal/backend/internal/domain"
)

func TestWebhookDeliveryIdentityAndLeaseRecovery(t *testing.T) {
	repo := NewMemory()
	now := time.Unix(100, 0).UTC()
	item := domain.WebhookDelivery{ID: "delivery-1", TenantID: "tenant-a", EventID: "event-a", Endpoint: "https://merchant.example/hook", Status: domain.WebhookDeliveryPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWebhookDelivery(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	item.ID = "different-id"
	if err := repo.CreateWebhookDelivery(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimWebhookDeliveries(context.Background(), "worker-a", now, time.Minute, 10)
	if err != nil || len(claimed) != 1 || claimed[0].AttemptCount != 1 {
		t.Fatalf("claim=%#v err=%v", claimed, err)
	}
	if recovered, err := repo.ClaimWebhookDeliveries(context.Background(), "worker-b", now.Add(2*time.Minute), time.Minute, 10); err != nil || len(recovered) != 1 || recovered[0].LockedBy != "worker-b" {
		t.Fatalf("recovery=%#v err=%v", recovered, err)
	}
}
