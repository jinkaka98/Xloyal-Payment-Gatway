package store

import (
	"context"
	"sort"
	"time"

	"xloyal/backend/internal/domain"
)

func (m *Memory) CreateWebhookDelivery(_ context.Context, delivery domain.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.webhookDeliveries {
		if existing.TenantID == delivery.TenantID && existing.EventID == delivery.EventID && existing.Endpoint == delivery.Endpoint {
			return nil
		}
	}
	if delivery.Status == "" {
		delivery.Status = domain.WebhookDeliveryPending
	}
	if delivery.NextAttemptAt.IsZero() {
		delivery.NextAttemptAt = delivery.CreatedAt
	}
	m.webhookDeliveries[delivery.ID] = delivery
	return nil
}

func (m *Memory) ClaimWebhookDeliveries(_ context.Context, owner string, now time.Time, lease time.Duration, limit int) ([]domain.WebhookDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, item := range m.webhookDeliveries {
		if item.Status == domain.WebhookDeliveryDelivering && item.LockedAt != nil && !item.LockedAt.Add(lease).After(now) {
			item.Status, item.LockedBy, item.LockedAt = domain.WebhookDeliveryRetrying, "", nil
			item.NextAttemptAt = now
			m.webhookDeliveries[id] = item
		}
	}
	ids := make([]string, 0)
	for id, item := range m.webhookDeliveries {
		if (item.Status == domain.WebhookDeliveryPending || item.Status == domain.WebhookDeliveryRetrying) && !item.NextAttemptAt.After(now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return m.webhookDeliveries[ids[i]].CreatedAt.Before(m.webhookDeliveries[ids[j]].CreatedAt)
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	result := make([]domain.WebhookDelivery, 0, len(ids))
	for _, id := range ids {
		item := m.webhookDeliveries[id]
		item.Status, item.LockedBy, item.LockedAt, item.AttemptCount = domain.WebhookDeliveryDelivering, owner, ptrTime(now), item.AttemptCount+1
		item.UpdatedAt = now
		m.webhookDeliveries[id] = item
		result = append(result, item)
	}
	return result, nil
}

func (m *Memory) MarkWebhookDelivered(_ context.Context, id, owner string, now time.Time, statusCode int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.webhookDeliveries[id]
	if !ok {
		return ErrNotFound
	}
	if item.Status != domain.WebhookDeliveryDelivering || item.LockedBy != owner {
		return ErrConflict
	}
	item.Status, item.LastStatusCode, item.LockedBy, item.LockedAt, item.DeliveredAt, item.UpdatedAt = domain.WebhookDeliveryDelivered, statusCode, "", nil, ptrTime(now), now
	m.webhookDeliveries[id] = item
	return nil
}

func (m *Memory) MarkWebhookRetry(_ context.Context, id, owner string, now time.Time, retry time.Duration, statusCode int, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.webhookDeliveries[id]
	if !ok {
		return ErrNotFound
	}
	if item.Status != domain.WebhookDeliveryDelivering || item.LockedBy != owner {
		return ErrConflict
	}
	item.Status, item.LastError, item.LastStatusCode, item.NextAttemptAt, item.LockedBy, item.LockedAt, item.UpdatedAt = domain.WebhookDeliveryRetrying, message, statusCode, now.Add(retry), "", nil, now
	m.webhookDeliveries[id] = item
	return nil
}

func (m *Memory) MarkWebhookFailed(_ context.Context, id, owner string, now time.Time, statusCode int, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.webhookDeliveries[id]
	if !ok {
		return ErrNotFound
	}
	if item.Status != domain.WebhookDeliveryDelivering || item.LockedBy != owner {
		return ErrConflict
	}
	item.Status, item.LastError, item.LastStatusCode, item.LockedBy, item.LockedAt, item.UpdatedAt = domain.WebhookDeliveryFailed, message, statusCode, "", nil, now
	m.webhookDeliveries[id] = item
	return nil
}

func ptrTime(value time.Time) *time.Time { return &value }
