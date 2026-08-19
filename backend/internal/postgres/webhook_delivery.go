package postgres

import (
	"context"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

const webhookDeliveryColumns = `id,tenant_id,event_id,event_type,payment_session_id,invoice_id,endpoint,payload,status,attempt_count,next_attempt_at,last_error,last_status_code,locked_at,locked_by,delivered_at,created_at,updated_at`

func scanWebhookDelivery(s interface{ Scan(...any) error }) (domain.WebhookDelivery, error) {
	var v domain.WebhookDelivery
	err := s.Scan(&v.ID, &v.TenantID, &v.EventID, &v.EventType, &v.PaymentSessionID, &v.InvoiceID, &v.Endpoint, &v.Payload, &v.Status, &v.AttemptCount, &v.NextAttemptAt, &v.LastError, &v.LastStatusCode, &v.LockedAt, &v.LockedBy, &v.DeliveredAt, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func (r *Repository) CreateWebhookDelivery(ctx context.Context, v domain.WebhookDelivery) error {
	if v.Status == "" {
		v.Status = domain.WebhookDeliveryPending
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO webhook_deliveries(`+webhookDeliveryColumns+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) ON CONFLICT (tenant_id,event_id,endpoint) DO NOTHING`, v.ID, v.TenantID, v.EventID, v.EventType, v.PaymentSessionID, v.InvoiceID, v.Endpoint, v.Payload, v.Status, v.AttemptCount, v.NextAttemptAt, v.LastError, v.LastStatusCode, v.LockedAt, v.LockedBy, v.DeliveredAt, v.CreatedAt, v.UpdatedAt)
	return err
}

func (r *Repository) ClaimWebhookDeliveries(ctx context.Context, owner string, now time.Time, lease time.Duration, limit int) ([]domain.WebhookDelivery, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE webhook_deliveries SET status='RETRYING',locked_at=NULL,locked_by='',next_attempt_at=$1,updated_at=$1 WHERE status='DELIVERING' AND locked_at <= $2`, now, now.Add(-lease)); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+webhookDeliveryColumns+` FROM webhook_deliveries WHERE status IN ('PENDING','RETRYING') AND next_attempt_at <= $1 ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.WebhookDelivery{}
	for rows.Next() {
		item, scanErr := scanWebhookDelivery(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		if _, err = tx.ExecContext(ctx, `UPDATE webhook_deliveries SET status='DELIVERING',attempt_count=attempt_count+1,locked_at=$1,locked_by=$2,updated_at=$1 WHERE id=$3`, now, owner, items[i].ID); err != nil {
			return nil, err
		}
		items[i].Status, items[i].AttemptCount, items[i].LockedAt, items[i].LockedBy, items[i].UpdatedAt = domain.WebhookDeliveryDelivering, items[i].AttemptCount+1, &now, owner, now
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) MarkWebhookDelivered(ctx context.Context, id, owner string, now time.Time, statusCode int) error {
	return r.updateWebhookTerminal(ctx, id, owner, now, domain.WebhookDeliveryDelivered, statusCode, "", true)
}

func (r *Repository) MarkWebhookRetry(ctx context.Context, id, owner string, now time.Time, retry time.Duration, statusCode int, message string) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE webhook_deliveries SET status='RETRYING',last_error=$1,last_status_code=$2,next_attempt_at=$3,locked_at=NULL,locked_by='',updated_at=$4 WHERE id=$5 AND status='DELIVERING' AND locked_by=$6`, message, statusCode, now.Add(retry), now, id, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}

func (r *Repository) MarkWebhookFailed(ctx context.Context, id, owner string, now time.Time, statusCode int, message string) error {
	return r.updateWebhookTerminal(ctx, id, owner, now, domain.WebhookDeliveryFailed, statusCode, message, false)
}

func (r *Repository) updateWebhookTerminal(ctx context.Context, id, owner string, now time.Time, status string, statusCode int, message string, delivered bool) error {
	query := `UPDATE webhook_deliveries SET status=$1,last_error=$2,last_status_code=$3,locked_at=NULL,locked_by='',updated_at=$4`
	if delivered {
		query += `,delivered_at=$4`
	}
	query += ` WHERE id=$5 AND status='DELIVERING' AND locked_by=$6`
	res, err := r.DB.ExecContext(ctx, query, status, message, statusCode, now, id, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}
