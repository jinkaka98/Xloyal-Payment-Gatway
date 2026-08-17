package store

import (
	"context"
	"testing"
	"time"

	"xloyal/backend/internal/domain"
)

func TestBrowserJobsCoalesceQueuedRequestsAndSerializeRunningResource(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
	first := domain.BrowserJob{ID: "job-a", ResourceKey: "neko-shared", MerchantID: "merchant-a", Kind: "merchant_sync", Priority: 50, State: "queued", NotBefore: now, RequestedAt: now}
	queued, created, err := repo.EnqueueBrowserJob(ctx, first)
	if err != nil || !created || queued.ID != first.ID {
		t.Fatalf("first enqueue created=%v job=%+v err=%v", created, queued, err)
	}
	duplicate := first
	duplicate.ID = "job-duplicate"
	duplicate.Priority = 80
	queued, created, err = repo.EnqueueBrowserJob(ctx, duplicate)
	if err != nil || created || queued.ID != first.ID || queued.RequestCount != 2 || queued.Priority != 80 {
		t.Fatalf("coalesced created=%v job=%+v err=%v", created, queued, err)
	}

	running, claimed, err := repo.ClaimBrowserJob(ctx, "worker-a", now, 2*time.Minute)
	if err != nil || !claimed || running.ID != first.ID || running.State != "running" {
		t.Fatalf("first claim claimed=%v job=%+v err=%v", claimed, running, err)
	}
	followup := first
	followup.ID = "job-followup"
	followup.RequestedAt = now.Add(time.Second)
	queued, created, err = repo.EnqueueBrowserJob(ctx, followup)
	if err != nil || !created || queued.ID != followup.ID {
		t.Fatalf("followup created=%v job=%+v err=%v", created, queued, err)
	}
	if job, claimed, err := repo.ClaimBrowserJob(ctx, "worker-b", now.Add(time.Second), 2*time.Minute); err != nil || claimed {
		t.Fatalf("parallel claim claimed=%v job=%+v err=%v", claimed, job, err)
	}
	if err := repo.CompleteBrowserJob(ctx, running.ID, "worker-a", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	job, claimed, err := repo.ClaimBrowserJob(ctx, "worker-b", now.Add(3*time.Second), 2*time.Minute)
	if err != nil || !claimed || job.ID != followup.ID {
		t.Fatalf("followup claim claimed=%v job=%+v err=%v", claimed, job, err)
	}
}

func TestBrowserJobExpiredLeaseIsRecovered(t *testing.T) {
	ctx := context.Background()
	repo := NewMemory()
	now := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	job := domain.BrowserJob{ID: "job-recover", ResourceKey: "neko-shared", Kind: "payment_validation", Priority: 100, State: "queued", NotBefore: now, RequestedAt: now}
	if _, _, err := repo.EnqueueBrowserJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimedJob, claimed, err := repo.ClaimBrowserJob(ctx, "worker-dead", now, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("initial claim job=%+v claimed=%v err=%v", claimedJob, claimed, err)
	}
	recovered, claimed, err := repo.ClaimBrowserJob(ctx, "worker-recovery", now.Add(2*time.Minute), time.Minute)
	if err != nil || !claimed || recovered.ID != job.ID || recovered.Attempt != 2 || recovered.LeaseOwner != "worker-recovery" {
		t.Fatalf("recovered job=%+v claimed=%v err=%v", recovered, claimed, err)
	}
}
