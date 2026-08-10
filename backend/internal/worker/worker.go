package worker

import (
	"context"
	"time"

	"xloyal/backend/internal/gateway"
	"xloyal/backend/internal/store"
)

const PollInterval = time.Minute
const MaxChecks = 30
const MaxAge = 30 * time.Minute

type Worker struct {
	Repo    store.Repository
	Gateway gateway.Service
	Now     func() time.Time
}

func (w Worker) RunOnce(ctx context.Context) error {
	now := w.now()
	items, err := w.Repo.PendingInvoices(ctx, now.Add(-PollInterval), 100)
	if err != nil {
		return err
	}
	for _, inv := range items {
		if inv.CheckCount >= MaxChecks || !inv.ExpiresAt.After(now) || now.Sub(inv.CreatedAt) >= MaxAge {
			if err := w.Gateway.Expire(ctx, inv); err != nil {
				return err
			}
			continue
		}
		if _, err := w.Gateway.Check(ctx, inv.TenantID, inv.ID); err != nil {
			continue
		}
	}
	return nil
}
func (w Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}
func (w Worker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
