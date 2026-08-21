package postgres

import (
	"context"
	"database/sql"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

func scanPaymentSession(s interface{ Scan(...any) error }) (domain.PaymentSession, error) {
	var v domain.PaymentSession
	err := s.Scan(&v.ID, &v.TenantID, &v.InvoiceID, &v.PublicTokenHash, &v.Status, &v.ThemeID, &v.ThemeVersion, &v.ReturnURL, &v.SuccessURL, &v.CancelURL, &v.FailedURL, &v.ExpiredURL, &v.ExpiresAt, &v.LastSeenAt, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

const paymentSessionColumns = `id,tenant_id,invoice_id,public_token_hash,status,COALESCE(theme_id,''),COALESCE(theme_version,0),COALESCE(return_url,''),COALESCE(success_url,''),COALESCE(cancel_url,''),COALESCE(failed_url,''),COALESCE(expired_url,''),expires_at,last_seen_at,created_at,updated_at`
const paymentSessionInsertColumns = `id,tenant_id,invoice_id,public_token_hash,status,theme_id,theme_version,return_url,success_url,cancel_url,failed_url,expired_url,expires_at,last_seen_at,created_at,updated_at`

func (r *Repository) CreatePaymentSession(ctx context.Context, session domain.PaymentSession, event domain.PaymentEvent, outbox domain.OutboxEvent) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var invoiceStatus string
	var invoiceExpiry time.Time
	if err = tx.QueryRowContext(ctx, `SELECT status,expires_at FROM invoices WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, session.InvoiceID, session.TenantID).Scan(&invoiceStatus, &invoiceExpiry); err != nil {
		return notFound(err)
	}
	if invoiceStatus != string(domain.InvoicePending) || session.ExpiresAt.After(invoiceExpiry) {
		return store.ErrConflict
	}
	if session.Status != domain.PaymentSessionOpen || event.EventID == "" || outbox.ID == "" {
		return store.ErrConflict
	}
	if (session.ThemeID == "") != (session.ThemeVersion == 0) {
		return store.ErrConflict
	}
	if session.ThemeID != "" {
		var themeTenant, themeStatus, versionStatus string
		var isDefault bool
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(t.tenant_id,''),t.status,t.is_default,tv.status
			FROM payment_theme_versions tv
			JOIN payment_themes t ON t.id=tv.theme_id
			WHERE tv.theme_id=$1 AND tv.version=$2
			FOR UPDATE OF tv,t`, session.ThemeID, session.ThemeVersion).
			Scan(&themeTenant, &themeStatus, &isDefault, &versionStatus)
		if err != nil {
			return notFound(err)
		}
		if themeStatus != domain.ThemePublished || versionStatus != domain.ThemePublished ||
			(themeTenant != session.TenantID && !(themeTenant == "" && isDefault)) {
			return store.ErrConflict
		}
	}
	redirects := []struct {
		kind string
		url  string
	}{
		{domain.RedirectSuccess, session.ReturnURL},
		{domain.RedirectSuccess, session.SuccessURL},
		{domain.RedirectCancel, session.CancelURL},
		{domain.RedirectFailed, session.FailedURL},
		{domain.RedirectExpired, session.ExpiredURL},
	}
	for _, redirect := range redirects {
		if redirect.url == "" {
			continue
		}
		var redirectID string
		if err = tx.QueryRowContext(ctx, `SELECT id FROM tenant_allowed_redirect_urls
			WHERE tenant_id=$1 AND url=$2 AND type=$3 AND active=true
			FOR UPDATE`, session.TenantID, redirect.url, redirect.kind).Scan(&redirectID); err != nil {
			return notFound(err)
		}
	}
	event.TenantID, event.InvoiceID, event.PaymentSessionID = session.TenantID, session.InvoiceID, session.ID
	outbox.EventID, outbox.TenantID, outbox.EventType, outbox.AggregateType, outbox.AggregateID = event.EventID, session.TenantID, event.EventType, "payment_session", session.ID
	if outbox.Status == "" {
		outbox.Status = domain.OutboxPending
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO payment_sessions(`+paymentSessionInsertColumns+`) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,0),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,$14,$15,$16)`, session.ID, session.TenantID, session.InvoiceID, session.PublicTokenHash, session.Status, session.ThemeID, session.ThemeVersion, session.ReturnURL, session.SuccessURL, session.CancelURL, session.FailedURL, session.ExpiredURL, session.ExpiresAt, session.LastSeenAt, session.CreatedAt, session.UpdatedAt); err != nil {
		return err
	}
	if err = insertPaymentEvent(ctx, tx, event); err != nil {
		return err
	}
	if err = insertOutbox(ctx, tx, outbox); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) PaymentSession(ctx context.Context, tenantID, id string) (domain.PaymentSession, error) {
	v, err := scanPaymentSession(r.DB.QueryRowContext(ctx, `SELECT `+paymentSessionColumns+` FROM payment_sessions WHERE id=$1 AND tenant_id=$2`, id, tenantID))
	return v, notFound(err)
}

func (r *Repository) PaymentSessionByTokenHash(ctx context.Context, hash string) (domain.PaymentSession, error) {
	v, err := scanPaymentSession(r.DB.QueryRowContext(ctx, `SELECT `+paymentSessionColumns+` FROM payment_sessions WHERE public_token_hash=$1`, hash))
	return v, notFound(err)
}

func (r *Repository) TransitionPaymentSession(ctx context.Context, tenantID, id string, next domain.PaymentSessionStatus, now time.Time, event domain.PaymentEvent, outbox domain.OutboxEvent) (domain.PaymentSession, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.PaymentSession{}, err
	}
	defer tx.Rollback()
	v, err := scanPaymentSession(tx.QueryRowContext(ctx, `SELECT `+paymentSessionColumns+` FROM payment_sessions WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, tenantID))
	if err != nil {
		return v, notFound(err)
	}
	if !v.Status.CanTransition(next) {
		return v, store.ErrConflict
	}
	expectedEventType, ok := v.Status.PaymentEventTypeForTransition(next)
	if !ok || event.EventType != expectedEventType {
		return v, store.ErrConflict
	}
	res, err := tx.ExecContext(ctx, `UPDATE payment_sessions SET status=$1,updated_at=$2 WHERE id=$3 AND tenant_id=$4 AND status=$5`, next, now, id, tenantID, v.Status)
	if err != nil {
		return v, err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return v, err
	}
	if changed != 1 {
		return v, store.ErrConflict
	}
	if invoiceStatus, terminal := next.InvoiceTerminalStatus(); terminal {
		invoiceResult, invoiceErr := tx.ExecContext(ctx, `UPDATE invoices SET status=$1,updated_at=$2 WHERE id=$3 AND tenant_id=$4 AND status=$5`, invoiceStatus, now, v.InvoiceID, tenantID, domain.InvoicePending)
		if invoiceErr != nil {
			return v, invoiceErr
		}
		invoiceChanged, invoiceErr := invoiceResult.RowsAffected()
		if invoiceErr != nil {
			return v, invoiceErr
		}
		if invoiceChanged != 1 {
			var currentInvoiceStatus string
			if err := tx.QueryRowContext(ctx, `SELECT status FROM invoices WHERE id=$1 AND tenant_id=$2 FOR SHARE`, v.InvoiceID, tenantID).Scan(&currentInvoiceStatus); err != nil || currentInvoiceStatus != string(invoiceStatus) {
				return v, store.ErrConflict
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hosted_invoice_unique_amount_reservations SET state='cooldown',terminal_status=$1,terminal_at=$2,cooldown_until=$2::timestamptz + (cooldown_minutes * INTERVAL '1 minute') WHERE invoice_id=$3 AND unique_amount_code > 0 AND state='active'`, invoiceStatus, now, v.InvoiceID); err != nil {
			return v, err
		}
	}
	v.Status, v.UpdatedAt = next, now
	if event.EventID == "" || outbox.ID == "" {
		return v, store.ErrConflict
	}
	event.TenantID, event.PaymentSessionID, event.InvoiceID = tenantID, id, v.InvoiceID
	outbox.EventID, outbox.TenantID, outbox.EventType, outbox.AggregateType, outbox.AggregateID = event.EventID, tenantID, event.EventType, "payment_session", id
	if outbox.Status == "" {
		outbox.Status = domain.OutboxPending
	}
	if err = insertPaymentEvent(ctx, tx, event); err != nil {
		return v, err
	}
	if err = insertOutbox(ctx, tx, outbox); err != nil {
		return v, err
	}
	if err = tx.Commit(); err != nil {
		return v, err
	}
	return v, nil
}

func insertPaymentEvent(ctx context.Context, tx *sql.Tx, event domain.PaymentEvent) error {
	if event.ID == "" || event.EventID == "" || event.TenantID == "" || event.InvoiceID == "" || event.PaymentSessionID == "" || !domain.IsPaymentEventType(event.EventType) {
		return store.ErrConflict
	}
	if event.Payload == nil {
		event.Payload = []byte(`{}`)
	}
	if event.SequenceNumber == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence_number),0)+1 FROM payment_events WHERE payment_session_id=$1`, event.PaymentSessionID).Scan(&event.SequenceNumber); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO payment_events(id,event_id,tenant_id,invoice_id,payment_session_id,sequence_number,event_type,payload,occurred_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, event.ID, event.EventID, event.TenantID, event.InvoiceID, event.PaymentSessionID, event.SequenceNumber, event.EventType, event.Payload, event.OccurredAt, event.CreatedAt)
	return err
}

func insertOutbox(ctx context.Context, tx *sql.Tx, event domain.OutboxEvent) error {
	if event.ID == "" || event.EventID == "" || event.TenantID == "" || event.EventType == "" || event.AggregateType == "" || event.AggregateID == "" {
		return store.ErrConflict
	}
	if event.Payload == nil {
		event.Payload = []byte(`{}`)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO outbox_events(id,event_id,tenant_id,event_type,aggregate_type,aggregate_id,payload,status,attempt_count,next_attempt_at,last_error,locked_at,locked_by,created_at,processed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, event.ID, event.EventID, event.TenantID, event.EventType, event.AggregateType, event.AggregateID, event.Payload, event.Status, event.AttemptCount, event.NextAttemptAt, event.LastError, event.LockedAt, event.LockedBy, event.CreatedAt, event.ProcessedAt)
	return err
}

func (r *Repository) ClaimOutboxEvents(ctx context.Context, owner string, now time.Time, lease time.Duration, limit int) ([]domain.OutboxEvent, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE outbox_events SET status='PENDING',locked_at=NULL,locked_by='' WHERE status='PROCESSING' AND locked_at <= $1`, now.Add(-lease)); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,event_id,tenant_id,event_type,aggregate_type,aggregate_id,payload,status,attempt_count,next_attempt_at,last_error,locked_at,locked_by,created_at,processed_at FROM outbox_events WHERE status='PENDING' AND next_attempt_at <= $1 ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OutboxEvent
	for rows.Next() {
		var v domain.OutboxEvent
		if err = rows.Scan(&v.ID, &v.EventID, &v.TenantID, &v.EventType, &v.AggregateType, &v.AggregateID, &v.Payload, &v.Status, &v.AttemptCount, &v.NextAttemptAt, &v.LastError, &v.LockedAt, &v.LockedBy, &v.CreatedAt, &v.ProcessedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		if _, err = tx.ExecContext(ctx, `UPDATE outbox_events SET status='PROCESSING',attempt_count=attempt_count+1,locked_at=$1,locked_by=$2 WHERE id=$3`, now, owner, out[i].ID); err != nil {
			return nil, err
		}
		out[i].Status, out[i].AttemptCount, out[i].LockedAt, out[i].LockedBy = "PROCESSING", out[i].AttemptCount+1, &now, owner
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) CompleteOutboxEvent(ctx context.Context, id, owner string, now time.Time) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE outbox_events SET status='DELIVERED',processed_at=$1,locked_at=NULL,locked_by='' WHERE id=$2 AND status='PROCESSING' AND locked_by=$3`, now, id, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}

func (r *Repository) FailOutboxEvent(ctx context.Context, id, errText string, now time.Time, retry time.Duration, owner string) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE outbox_events SET status='PENDING',last_error=$1,next_attempt_at=$2,locked_at=NULL,locked_by='' WHERE id=$3 AND status='PROCESSING' AND locked_by=$4`, errText, now.Add(retry), id, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}

func (r *Repository) RedirectURLAllowed(ctx context.Context, tenantID, url, kind string) (bool, error) {
	var ok bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tenant_allowed_redirect_urls WHERE tenant_id=$1 AND url=$2 AND type=$3 AND active=true)`, tenantID, url, kind).Scan(&ok)
	return ok, err
}

func (r *Repository) PublishedPaymentThemeVersion(ctx context.Context, tenantID, themeID string, version int) (domain.PaymentThemeVersion, error) {
	var v domain.PaymentThemeVersion
	query := `SELECT tv.id,tv.theme_id,tv.version,tv.status,tv.config,tv.created_at FROM payment_theme_versions tv JOIN payment_themes t ON t.id=tv.theme_id WHERE (t.tenant_id=$1 OR t.tenant_id IS NULL) AND t.id=$2 AND t.status='PUBLISHED' AND tv.status='PUBLISHED' ORDER BY (t.tenant_id IS NULL),tv.version DESC LIMIT 1`
	args := []any{tenantID, themeID}
	if version > 0 {
		query += ` AND tv.version=$3`
		args = append(args, version)
	} else {
		query += ` ORDER BY tv.version DESC LIMIT 1`
	}
	err := r.DB.QueryRowContext(ctx, query, args...).Scan(&v.ID, &v.ThemeID, &v.Version, &v.Status, &v.Config, &v.CreatedAt)
	return v, notFound(err)
}

func (r *Repository) DefaultPublishedPaymentThemeVersion(ctx context.Context, tenantID string) (domain.PaymentThemeVersion, error) {
	var v domain.PaymentThemeVersion
	err := r.DB.QueryRowContext(ctx, `SELECT tv.id,tv.theme_id,tv.version,tv.status,tv.config,tv.created_at FROM payment_theme_versions tv JOIN payment_themes t ON t.id=tv.theme_id WHERE (t.tenant_id=$1 OR t.tenant_id IS NULL) AND t.is_default=true AND t.status='PUBLISHED' AND tv.status='PUBLISHED' ORDER BY (t.tenant_id IS NULL),tv.version DESC LIMIT 1`, tenantID).Scan(&v.ID, &v.ThemeID, &v.Version, &v.Status, &v.Config, &v.CreatedAt)
	return v, notFound(err)
}

func (r *Repository) PaymentThemeVersion(ctx context.Context, themeID string, version int) (domain.PaymentThemeVersion, error) {
	var v domain.PaymentThemeVersion
	err := r.DB.QueryRowContext(ctx, `SELECT id,theme_id,version,status,config,created_at FROM payment_theme_versions WHERE theme_id=$1 AND version=$2 AND status IN ('PUBLISHED','ARCHIVED')`, themeID, version).Scan(&v.ID, &v.ThemeID, &v.Version, &v.Status, &v.Config, &v.CreatedAt)
	return v, notFound(err)
}

func (r *Repository) TouchPaymentSession(ctx context.Context, tenantID, id string, now time.Time) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE payment_sessions SET last_seen_at=$1,updated_at=$1 WHERE id=$2 AND tenant_id=$3`, now, id, tenantID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *Repository) CreatePaymentEvent(ctx context.Context, event domain.PaymentEvent) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sessionTenant, invoiceID string
	if err = tx.QueryRowContext(ctx, `SELECT tenant_id,invoice_id FROM payment_sessions WHERE id=$1 FOR UPDATE`, event.PaymentSessionID).Scan(&sessionTenant, &invoiceID); err != nil {
		return notFound(err)
	}
	if event.TenantID != "" && event.TenantID != sessionTenant {
		return store.ErrConflict
	}
	if event.InvoiceID != "" && event.InvoiceID != invoiceID {
		return store.ErrConflict
	}
	event.TenantID, event.InvoiceID = sessionTenant, invoiceID
	if err = insertPaymentEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) PaymentEvents(ctx context.Context, tenantID, sessionID string) ([]domain.PaymentEvent, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,event_id,tenant_id,invoice_id,payment_session_id,sequence_number,event_type,payload,occurred_at,created_at FROM payment_events WHERE tenant_id=$1 AND payment_session_id=$2 ORDER BY sequence_number`, tenantID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.PaymentEvent, 0)
	for rows.Next() {
		var v domain.PaymentEvent
		if err = rows.Scan(&v.ID, &v.EventID, &v.TenantID, &v.InvoiceID, &v.PaymentSessionID, &v.SequenceNumber, &v.EventType, &v.Payload, &v.OccurredAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) CreateOutboxEvent(ctx context.Context, event domain.OutboxEvent) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eventTenant, eventType, sessionID string
	if err = tx.QueryRowContext(ctx, `SELECT tenant_id,event_type,payment_session_id FROM payment_events WHERE event_id=$1 FOR SHARE`, event.EventID).Scan(&eventTenant, &eventType, &sessionID); err != nil {
		return notFound(err)
	}
	if (event.TenantID != "" && event.TenantID != eventTenant) ||
		(event.EventType != "" && event.EventType != eventType) ||
		(event.AggregateType != "" && event.AggregateType != "payment_session") ||
		(event.AggregateID != "" && event.AggregateID != sessionID) {
		return store.ErrConflict
	}
	event.TenantID, event.EventType, event.AggregateType, event.AggregateID = eventTenant, eventType, "payment_session", sessionID
	if err = insertOutbox(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) MarkOutboxDelivered(ctx context.Context, id, owner string, now time.Time) error {
	return r.CompleteOutboxEvent(ctx, id, owner, now)
}
func (r *Repository) MarkOutboxRetry(ctx context.Context, id, owner string, now time.Time, retry time.Duration, errText string) error {
	return r.FailOutboxEvent(ctx, id, errText, now, retry, owner)
}
func (r *Repository) MarkOutboxFailed(ctx context.Context, id, owner string, now time.Time, errText string) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE outbox_events SET status='FAILED',last_error=$1,processed_at=$2,locked_at=NULL,locked_by='' WHERE id=$3 AND status='PROCESSING' AND locked_by=$4`, errText, now, id, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}
