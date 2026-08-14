package store

import (
	"context"
	"errors"
	"testing"

	"xloyal/backend/internal/domain"
)

func TestTenantMetadataUpdateCannotRestoreStaleCredential(t *testing.T) {
	repo := NewMemory()
	ctx := context.Background()
	original := domain.Tenant{ID: "tenant-a", Name: "Old", APIKeyHash: "old-hash", APIKeyCiphertext: "old-cipher", Active: true}
	if err := repo.CreateTenant(ctx, original); err != nil {
		t.Fatal(err)
	}
	stale, _ := repo.Tenant(ctx, original.ID)
	if err := repo.RotateTenantAPIKey(ctx, original.ID, original.APIKeyHash, "new-hash", "new-cipher", domain.AuditEvent{ID: "rotate-1"}); err != nil {
		t.Fatal(err)
	}
	stale.Name = "Updated"
	if err := repo.UpdateTenant(ctx, stale); err != nil {
		t.Fatal(err)
	}
	stored, _ := repo.Tenant(ctx, original.ID)
	if stored.APIKeyHash != "new-hash" || stored.APIKeyCiphertext != "new-cipher" {
		t.Fatalf("metadata update restored stale credential: %+v", stored)
	}
}

func TestTenantRotationRejectsStaleExpectedHash(t *testing.T) {
	repo := NewMemory()
	ctx := context.Background()
	repo.CreateTenant(ctx, domain.Tenant{ID: "tenant-a", APIKeyHash: "old-hash", Active: true})
	if err := repo.RotateTenantAPIKey(ctx, "tenant-a", "old-hash", "new-hash", "new-cipher", domain.AuditEvent{ID: "rotate-1"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.RotateTenantAPIKey(ctx, "tenant-a", "old-hash", "other-hash", "other-cipher", domain.AuditEvent{ID: "rotate-2"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale rotation error=%v", err)
	}
}
