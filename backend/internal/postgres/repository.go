package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

type Repository struct{ DB *sql.DB }

var _ store.Repository = (*Repository)(nil)

func New(db *sql.DB) *Repository                       { return &Repository{DB: db} }
func (r *Repository) Health(ctx context.Context) error { return r.DB.PingContext(ctx) }
func (r *Repository) TenantByAPIKey(ctx context.Context, h string) (domain.Tenant, error) {
	var v domain.Tenant
	err := r.DB.QueryRowContext(ctx, `SELECT id,COALESCE(merchant_id,''),name,COALESCE(site_url,''),COALESCE(callback_url,''),COALESCE(webhook_url,''),sandbox_mode,use_unique_amount_code,unique_amount_cooldown_minutes,api_key_hash,COALESCE(api_key_ciphertext,''),COALESCE(webhook_secret_ciphertext,''),webhook_replay_window_seconds,active,created_at FROM tenants WHERE api_key_hash=$1 AND active=true AND deleted_at IS NULL`, h).Scan(&v.ID, &v.MerchantID, &v.Name, &v.SiteURL, &v.CallbackURL, &v.WebhookURL, &v.SandboxMode, &v.UseUniqueAmountCode, &v.UniqueAmountCooldownMinutes, &v.APIKeyHash, &v.APIKeyCiphertext, &v.WebhookSecretCiphertext, &v.WebhookReplayWindowSeconds, &v.Active, &v.CreatedAt)
	v.APIKeyRecoverable = v.APIKeyCiphertext != ""
	v.WebhookSecretConfigured = v.WebhookSecretCiphertext != ""
	return v, notFound(err)
}
func (r *Repository) Tenant(ctx context.Context, id string) (domain.Tenant, error) {
	var v domain.Tenant
	err := r.DB.QueryRowContext(ctx, `SELECT id,COALESCE(merchant_id,''),name,COALESCE(site_url,''),COALESCE(callback_url,''),COALESCE(webhook_url,''),sandbox_mode,use_unique_amount_code,unique_amount_cooldown_minutes,api_key_hash,COALESCE(api_key_ciphertext,''),COALESCE(webhook_secret_ciphertext,''),webhook_replay_window_seconds,active,created_at FROM tenants WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&v.ID, &v.MerchantID, &v.Name, &v.SiteURL, &v.CallbackURL, &v.WebhookURL, &v.SandboxMode, &v.UseUniqueAmountCode, &v.UniqueAmountCooldownMinutes, &v.APIKeyHash, &v.APIKeyCiphertext, &v.WebhookSecretCiphertext, &v.WebhookReplayWindowSeconds, &v.Active, &v.CreatedAt)
	v.APIKeyRecoverable = v.APIKeyCiphertext != ""
	v.WebhookSecretConfigured = v.WebhookSecretCiphertext != ""
	return v, notFound(err)
}
func (r *Repository) CreateTenant(ctx context.Context, v domain.Tenant) error {
	v.UniqueAmountCooldownMinutes = normalizedCooldownMinutes(v.UniqueAmountCooldownMinutes)
	if v.WebhookReplayWindowSeconds == 0 {
		v.WebhookReplayWindowSeconds = 300
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO tenants(id,merchant_id,name,site_url,callback_url,webhook_url,sandbox_mode,use_unique_amount_code,unique_amount_cooldown_minutes,api_key_hash,api_key_ciphertext,webhook_secret_ciphertext,webhook_replay_window_seconds,active,created_at) VALUES($1,NULLIF($2,''),$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),$13,$14,$15)`, v.ID, v.MerchantID, v.Name, v.SiteURL, v.CallbackURL, v.WebhookURL, v.SandboxMode, v.UseUniqueAmountCode, v.UniqueAmountCooldownMinutes, v.APIKeyHash, v.APIKeyCiphertext, v.WebhookSecretCiphertext, v.WebhookReplayWindowSeconds, v.Active, v.CreatedAt)
	return err
}
func (r *Repository) UpdateTenant(ctx context.Context, v domain.Tenant) error {
	v.UniqueAmountCooldownMinutes = normalizedCooldownMinutes(v.UniqueAmountCooldownMinutes)
	if v.WebhookReplayWindowSeconds == 0 {
		v.WebhookReplayWindowSeconds = 300
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE tenants SET merchant_id=NULLIF($1,''),name=$2,site_url=NULLIF($3,''),callback_url=NULLIF($4,''),webhook_url=NULLIF($5,''),sandbox_mode=$6,use_unique_amount_code=$7,unique_amount_cooldown_minutes=$8,active=$9,webhook_replay_window_seconds=$10 WHERE id=$11`, v.MerchantID, v.Name, v.SiteURL, v.CallbackURL, v.WebhookURL, v.SandboxMode, v.UseUniqueAmountCode, v.UniqueAmountCooldownMinutes, v.Active, v.WebhookReplayWindowSeconds, v.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
func (r *Repository) DeleteTenant(ctx context.Context, id string, audit domain.AuditEvent) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE tenants SET active=false,api_key_hash=$1,api_key_ciphertext=NULL,webhook_secret_ciphertext=NULL,deleted_at=$2 WHERE id=$3 AND deleted_at IS NULL`, "deleted:"+audit.ID, audit.CreatedAt, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	meta, err := json.Marshal(audit.Metadata)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor,action,resource_type,resource_id,metadata,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, audit.ID, audit.TenantID, audit.Actor, audit.Action, audit.ResourceType, audit.ResourceID, meta, audit.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) RotateTenantAPIKey(ctx context.Context, tenantID, expectedHash, hash, ciphertext string, audit domain.AuditEvent) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE tenants SET api_key_hash=$1,api_key_ciphertext=$2 WHERE id=$3 AND api_key_hash=$4`, hash, ciphertext, tenantID, expectedHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1)`, tenantID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return store.ErrConflict
		}
		return store.ErrNotFound
	}
	meta, err := json.Marshal(audit.Metadata)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor,action,resource_type,resource_id,metadata,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, audit.ID, null(audit.TenantID), audit.Actor, audit.Action, audit.ResourceType, audit.ResourceID, meta, audit.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) RotateTenantWebhookSecret(ctx context.Context, tenantID, ciphertext string, audit domain.AuditEvent) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE tenants SET webhook_secret_ciphertext=$1 WHERE id=$2 AND deleted_at IS NULL`, ciphertext, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	meta, err := json.Marshal(audit.Metadata)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor,action,resource_type,resource_id,metadata,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, audit.ID, null(audit.TenantID), audit.Actor, audit.Action, audit.ResourceType, audit.ResourceID, meta, audit.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,COALESCE(merchant_id,''),name,COALESCE(site_url,''),COALESCE(callback_url,''),COALESCE(webhook_url,''),sandbox_mode,use_unique_amount_code,unique_amount_cooldown_minutes,api_key_hash,COALESCE(api_key_ciphertext,''),COALESCE(webhook_secret_ciphertext,''),webhook_replay_window_seconds,active,created_at FROM tenants WHERE deleted_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Tenant, 0)
	for rows.Next() {
		var v domain.Tenant
		if err := rows.Scan(&v.ID, &v.MerchantID, &v.Name, &v.SiteURL, &v.CallbackURL, &v.WebhookURL, &v.SandboxMode, &v.UseUniqueAmountCode, &v.UniqueAmountCooldownMinutes, &v.APIKeyHash, &v.APIKeyCiphertext, &v.WebhookSecretCiphertext, &v.WebhookReplayWindowSeconds, &v.Active, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.APIKeyRecoverable = v.APIKeyCiphertext != ""
		v.WebhookSecretConfigured = v.WebhookSecretCiphertext != ""
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) AssignTenantMerchant(ctx context.Context, tenantID, merchantID string) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE tenants SET merchant_id=$1 WHERE id=$2`, merchantID, tenantID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
func (r *Repository) CreateMerchantID(ctx context.Context, v domain.MerchantID) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO merchant_ids(id,interactive_merchant_id,name,credential_ciphertext,active,created_at) VALUES($1,$2,$3,$4,$5,$6)`, v.ID, v.InteractiveMerchantID, v.Name, v.CredentialCiphertext, v.Active, v.CreatedAt)
	return err
}
func (r *Repository) MerchantID(ctx context.Context, id string) (domain.MerchantID, error) {
	var v domain.MerchantID
	err := r.DB.QueryRowContext(ctx, `SELECT id,interactive_merchant_id,name,credential_ciphertext,active,created_at FROM merchant_ids WHERE id=$1`, id).Scan(&v.ID, &v.InteractiveMerchantID, &v.Name, &v.CredentialCiphertext, &v.Active, &v.CreatedAt)
	return v, notFound(err)
}
func (r *Repository) ListMerchantIDs(ctx context.Context) ([]domain.MerchantID, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,interactive_merchant_id,name,credential_ciphertext,active,created_at FROM merchant_ids ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MerchantID{}
	for rows.Next() {
		var v domain.MerchantID
		if err := rows.Scan(&v.ID, &v.InteractiveMerchantID, &v.Name, &v.CredentialCiphertext, &v.Active, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UpsertMerchantConnection(ctx context.Context, v domain.MerchantConnection) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO merchant_connections(merchant_id,session_ciphertext,browser_credential_ciphertext,status,last_synced_at,history_backfilled_at,last_error,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(merchant_id) DO UPDATE SET session_ciphertext=EXCLUDED.session_ciphertext,browser_credential_ciphertext=EXCLUDED.browser_credential_ciphertext,status=EXCLUDED.status,last_synced_at=EXCLUDED.last_synced_at,history_backfilled_at=EXCLUDED.history_backfilled_at,last_error=EXCLUDED.last_error,updated_at=EXCLUDED.updated_at`, v.MerchantID, v.SessionCiphertext, v.BrowserCredentialCiphertext, v.Status, v.LastSyncedAt, v.HistoryBackfilledAt, v.LastError, v.UpdatedAt)
	return err
}
func (r *Repository) MerchantConnection(ctx context.Context, id string) (domain.MerchantConnection, error) {
	var v domain.MerchantConnection
	err := r.DB.QueryRowContext(ctx, `SELECT merchant_id,session_ciphertext,browser_credential_ciphertext,status,last_synced_at,history_backfilled_at,last_error,updated_at FROM merchant_connections WHERE merchant_id=$1`, id).Scan(&v.MerchantID, &v.SessionCiphertext, &v.BrowserCredentialCiphertext, &v.Status, &v.LastSyncedAt, &v.HistoryBackfilledAt, &v.LastError, &v.UpdatedAt)
	return v, notFound(err)
}
func (r *Repository) ListDueMerchantConnections(ctx context.Context, due time.Time, limit int) ([]domain.MerchantConnection, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT merchant_id,session_ciphertext,browser_credential_ciphertext,status,last_synced_at,history_backfilled_at,last_error,updated_at FROM merchant_connections WHERE status IN ('connected','reconnect_required') AND last_error <> 'Manual browser login in progress' AND GREATEST(COALESCE(last_synced_at,updated_at),updated_at) <= $1 ORDER BY updated_at LIMIT $2`, due, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MerchantConnection{}
	for rows.Next() {
		var v domain.MerchantConnection
		if err := rows.Scan(&v.MerchantID, &v.SessionCiphertext, &v.BrowserCredentialCiphertext, &v.Status, &v.LastSyncedAt, &v.HistoryBackfilledAt, &v.LastError, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const browserJobColumns = `id,resource_key,merchant_id,kind,priority,state,not_before,requested_at,request_count,attempt,lease_owner,lease_until,started_at,completed_at,last_error`

func (r *Repository) EnqueueBrowserJob(ctx context.Context, job domain.BrowserJob) (domain.BrowserJob, bool, error) {
	if job.State == "" {
		job.State = "queued"
	}
	if job.RequestCount == 0 {
		job.RequestCount = 1
	}
	var created bool
	query := `INSERT INTO browser_jobs(` + browserJobColumns + `) VALUES($1,$2,$3,$4,$5,'queued',$6,$7,$8,0,'',NULL,NULL,NULL,'')
		ON CONFLICT(resource_key,merchant_id,kind) WHERE state='queued' DO UPDATE SET
		priority=GREATEST(browser_jobs.priority,EXCLUDED.priority),not_before=LEAST(browser_jobs.not_before,EXCLUDED.not_before),request_count=browser_jobs.request_count+1
		RETURNING ` + browserJobColumns + `,(xmax=0)`
	err := r.DB.QueryRowContext(ctx, query, job.ID, job.ResourceKey, job.MerchantID, job.Kind, job.Priority, job.NotBefore, job.RequestedAt, job.RequestCount).Scan(&job.ID, &job.ResourceKey, &job.MerchantID, &job.Kind, &job.Priority, &job.State, &job.NotBefore, &job.RequestedAt, &job.RequestCount, &job.Attempt, &job.LeaseOwner, &job.LeaseUntil, &job.StartedAt, &job.CompletedAt, &job.LastError, &created)
	return job, created, err
}

func (r *Repository) ClaimBrowserJob(ctx context.Context, owner string, now time.Time, lease time.Duration) (domain.BrowserJob, bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.BrowserJob{}, false, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,resource_key,merchant_id,kind,priority,not_before,request_count FROM browser_jobs WHERE state='running' AND lease_until <= $1 FOR UPDATE`, now)
	if err != nil {
		return domain.BrowserJob{}, false, err
	}
	expiredJobs := make([]domain.BrowserJob, 0)
	for rows.Next() {
		var expired domain.BrowserJob
		if err = rows.Scan(&expired.ID, &expired.ResourceKey, &expired.MerchantID, &expired.Kind, &expired.Priority, &expired.NotBefore, &expired.RequestCount); err != nil {
			rows.Close()
			return domain.BrowserJob{}, false, err
		}
		expiredJobs = append(expiredJobs, expired)
	}
	if err = rows.Close(); err != nil {
		return domain.BrowserJob{}, false, err
	}
	for _, expired := range expiredJobs {
		var merged int64
		err = tx.QueryRowContext(ctx, `UPDATE browser_jobs SET priority=GREATEST(priority,$1),not_before=LEAST(not_before,$2),request_count=request_count+$3,last_error='browser job lease expired; recovered' WHERE state='queued' AND resource_key=$4 AND merchant_id=$5 AND kind=$6 RETURNING 1`, expired.Priority, expired.NotBefore, expired.RequestCount, expired.ResourceKey, expired.MerchantID, expired.Kind).Scan(&merged)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `UPDATE browser_jobs SET state='queued',lease_owner='',lease_until=NULL,not_before=$1,last_error='browser job lease expired; recovered' WHERE id=$2`, now, expired.ID)
		} else if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM browser_jobs WHERE id=$1`, expired.ID)
		}
		if err != nil {
			return domain.BrowserJob{}, false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM browser_jobs WHERE state IN ('succeeded','failed') AND completed_at < $1`, now.Add(-7*24*time.Hour)); err != nil {
		return domain.BrowserJob{}, false, err
	}
	var job domain.BrowserJob
	err = tx.QueryRowContext(ctx, `SELECT `+browserJobColumns+` FROM browser_jobs candidate WHERE state='queued' AND not_before <= $1 AND NOT EXISTS (SELECT 1 FROM browser_jobs running WHERE running.resource_key=candidate.resource_key AND running.state='running') ORDER BY priority DESC,not_before,requested_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, now).Scan(&job.ID, &job.ResourceKey, &job.MerchantID, &job.Kind, &job.Priority, &job.State, &job.NotBefore, &job.RequestedAt, &job.RequestCount, &job.Attempt, &job.LeaseOwner, &job.LeaseUntil, &job.StartedAt, &job.CompletedAt, &job.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BrowserJob{}, false, nil
	}
	if err != nil {
		return domain.BrowserJob{}, false, err
	}
	leaseUntil := now.Add(lease)
	err = tx.QueryRowContext(ctx, `UPDATE browser_jobs SET state='running',lease_owner=$1,lease_until=$2,started_at=$3,completed_at=NULL,attempt=attempt+1 WHERE id=$4 AND state='queued' RETURNING `+browserJobColumns, owner, leaseUntil, now, job.ID).Scan(&job.ID, &job.ResourceKey, &job.MerchantID, &job.Kind, &job.Priority, &job.State, &job.NotBefore, &job.RequestedAt, &job.RequestCount, &job.Attempt, &job.LeaseOwner, &job.LeaseUntil, &job.StartedAt, &job.CompletedAt, &job.LastError)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "browser_jobs_running_resource_idx" {
			return domain.BrowserJob{}, false, nil
		}
		return domain.BrowserJob{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return domain.BrowserJob{}, false, err
	}
	return job, true, nil
}

func (r *Repository) CompleteBrowserJob(ctx context.Context, id, owner string, completedAt time.Time) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE browser_jobs SET state='succeeded',completed_at=$1,lease_owner='',lease_until=NULL,last_error='' WHERE id=$2 AND state='running' AND lease_owner=$3`, completedAt, id, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}

func (r *Repository) FailBrowserJob(ctx context.Context, id, owner string, completedAt time.Time, message string) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE browser_jobs SET state='failed',completed_at=$1,lease_owner='',lease_until=NULL,last_error=$2 WHERE id=$3 AND state='running' AND lease_owner=$4`, completedAt, message, id, owner)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}
func (r *Repository) CreatePortalTransaction(ctx context.Context, v domain.PortalTransaction) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO portal_transactions(id,merchant_id,tenant_id,reference,amount,status,paid_at,source,match_confidence,invoice_id,created_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11) ON CONFLICT(merchant_id,reference) DO UPDATE SET amount=EXCLUDED.amount,status=EXCLUDED.status,paid_at=EXCLUDED.paid_at,source=EXCLUDED.source,tenant_id=COALESCE(portal_transactions.tenant_id,EXCLUDED.tenant_id),invoice_id=COALESCE(portal_transactions.invoice_id,EXCLUDED.invoice_id),match_confidence=CASE WHEN portal_transactions.tenant_id IS NULL THEN EXCLUDED.match_confidence ELSE portal_transactions.match_confidence END`, v.ID, v.MerchantID, v.TenantID, v.Reference, v.Amount, v.Status, v.PaidAt, v.Source, v.MatchConfidence, v.InvoiceID, v.CreatedAt)
	return err
}
func (r *Repository) ListPortalTransactions(ctx context.Context, merchantID, tenantID string, limit int) ([]domain.PortalTransaction, error) {
	q := `SELECT id,merchant_id,COALESCE(tenant_id,''),reference,amount,status,paid_at,source,match_confidence,COALESCE(invoice_id,''),created_at FROM portal_transactions`
	args := []any{}
	where := []string{}
	if merchantID != "" {
		args = append(args, merchantID)
		where = append(where, `merchant_id=$`+fmt.Sprint(len(args)))
	}
	if tenantID != "" {
		args = append(args, tenantID)
		where = append(where, `tenant_id=$`+fmt.Sprint(len(args)))
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	args = append(args, limit)
	q += ` ORDER BY paid_at DESC LIMIT $` + fmt.Sprint(len(args))
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PortalTransaction{}
	for rows.Next() {
		var v domain.PortalTransaction
		if err := rows.Scan(&v.ID, &v.MerchantID, &v.TenantID, &v.Reference, &v.Amount, &v.Status, &v.PaidAt, &v.Source, &v.MatchConfidence, &v.InvoiceID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UpsertTariff(ctx context.Context, v domain.Tariff) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO merchant_tariffs(merchant_id,basis_points,fixed_fee,active,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(merchant_id) DO UPDATE SET basis_points=EXCLUDED.basis_points,fixed_fee=EXCLUDED.fixed_fee,active=EXCLUDED.active,updated_at=EXCLUDED.updated_at`, v.MerchantID, v.BasisPoints, v.FixedFee, v.Active, v.UpdatedAt)
	return err
}
func (r *Repository) Tariff(ctx context.Context, merchantID string) (domain.Tariff, error) {
	var v domain.Tariff
	err := r.DB.QueryRowContext(ctx, `SELECT merchant_id,basis_points,fixed_fee,active,updated_at FROM merchant_tariffs WHERE merchant_id=$1`, merchantID).Scan(&v.MerchantID, &v.BasisPoints, &v.FixedFee, &v.Active, &v.UpdatedAt)
	return v, notFound(err)
}
func (r *Repository) CreateMerchantAccount(ctx context.Context, v domain.MerchantAccount) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO merchant_accounts(id,tenant_id,provider,name,credential_ciphertext,active,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, v.ID, v.TenantID, v.Provider, v.Name, v.CredentialCiphertext, v.Active, v.CreatedAt)
	return err
}
func (r *Repository) MerchantAccount(ctx context.Context, tenant, id string) (domain.MerchantAccount, error) {
	var v domain.MerchantAccount
	err := r.DB.QueryRowContext(ctx, `SELECT id,tenant_id,provider,name,credential_ciphertext,active,created_at FROM merchant_accounts WHERE id=$1 AND tenant_id=$2`, id, tenant).Scan(&v.ID, &v.TenantID, &v.Provider, &v.Name, &v.CredentialCiphertext, &v.Active, &v.CreatedAt)
	return v, notFound(err)
}
func (r *Repository) UpdateMerchantAccount(ctx context.Context, v domain.MerchantAccount) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE merchant_accounts SET tenant_id=$2, provider=$3, name=$4, credential_ciphertext=$5, active=$6 WHERE id=$1`, v.ID, v.TenantID, v.Provider, v.Name, v.CredentialCiphertext, v.Active)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
func (r *Repository) ListMerchantAccounts(ctx context.Context, tenant string) ([]domain.MerchantAccount, error) {
	q := `SELECT id,tenant_id,provider,name,credential_ciphertext,active,created_at FROM merchant_accounts`
	args := []any{}
	if tenant != "" {
		q += ` WHERE tenant_id=$1`
		args = append(args, tenant)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.MerchantAccount, 0)
	for rows.Next() {
		var v domain.MerchantAccount
		if err := rows.Scan(&v.ID, &v.TenantID, &v.Provider, &v.Name, &v.CredentialCiphertext, &v.Active, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) CreateInvoice(ctx context.Context, v domain.Invoice) (domain.Invoice, bool, error) {
	// Keep compatibility with invoice producers predating requested_amount.
	if v.RequestedAmount <= 0 {
		v.RequestedAmount = v.Amount
	}
	res, err := r.DB.ExecContext(ctx, `INSERT INTO invoices(id,tenant_id,merchant_account_id,idempotency_key,requested_amount,amount,unique_amount_code,qris_template_id,qris_merchant_id,qris_merchant_name,qris_merchant_city,currency,description,provider_reference,provider_request_date,qr_payload,status,created_at,updated_at,expires_at,check_count,sandbox_mode) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22) ON CONFLICT(tenant_id,idempotency_key) DO NOTHING`, v.ID, v.TenantID, v.MerchantAccountID, v.IdempotencyKey, v.RequestedAmount, v.Amount, v.UniqueAmountCode, v.QRISTemplateID, v.QRISMerchantID, v.QRISMerchantName, v.QRISMerchantCity, v.Currency, v.Description, v.ProviderReference, v.ProviderRequestDate, v.QRPayload, v.Status, v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.CheckCount, v.SandboxMode)
	if err != nil {
		return v, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return v, false, err
	}
	if n == 1 {
		return v, true, nil
	}
	existing, err := r.invoiceByIdempotency(ctx, v.TenantID, v.IdempotencyKey)
	return existing, false, err
}
func (r *Repository) ActivateHostedInvoice(ctx context.Context, v domain.Invoice) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM hosted_invoice_unique_amount_reservations WHERE (state='cooldown' AND cooldown_until <= $1) OR (state='active' AND expires_at <= $1)`, v.CreatedAt); err != nil {
		return err
	}
	if v.UniqueAmountCode > 0 {
		var cooldown int
		if err = tx.QueryRowContext(ctx, `SELECT unique_amount_cooldown_minutes FROM tenants WHERE id=$1`, v.TenantID).Scan(&cooldown); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO hosted_invoice_unique_amount_reservations(invoice_id,tenant_id,merchant_id,payable_amount,unique_amount_code,expires_at,reserved_at,cooldown_minutes) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.TenantID, v.QRISMerchantID, v.Amount, v.UniqueAmountCode, v.ExpiresAt, v.CreatedAt, cooldown)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return store.ErrUniqueAmountUnavailable
			}
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE invoices SET requested_amount=$1,amount=$2,unique_amount_code=$3,qris_template_id=NULLIF($4,''),qris_merchant_id=NULLIF($5,''),qris_merchant_name=NULLIF($6,''),qris_merchant_city=NULLIF($7,''),provider_reference=$8,provider_request_date=$9,qr_payload=$10,status=$11,updated_at=$12 WHERE id=$13 AND tenant_id=$14 AND status='creating'`, v.RequestedAmount, v.Amount, v.UniqueAmountCode, v.QRISTemplateID, v.QRISMerchantID, v.QRISMerchantName, v.QRISMerchantCity, v.ProviderReference, v.ProviderRequestDate, v.QRPayload, v.Status, v.UpdatedAt, v.ID, v.TenantID)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return store.ErrConflict
	}
	return tx.Commit()
}
func (r *Repository) invoiceByIdempotency(ctx context.Context, tenant, key string) (domain.Invoice, error) {
	return scanInvoice(r.DB.QueryRowContext(ctx, invoiceColumns+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenant, key))
}
func (r *Repository) Invoice(ctx context.Context, tenant, id string) (domain.Invoice, error) {
	if tenant == "" {
		return scanInvoice(r.DB.QueryRowContext(ctx, invoiceColumns+` WHERE id=$1`, id))
	}
	return scanInvoice(r.DB.QueryRowContext(ctx, invoiceColumns+` WHERE id=$1 AND tenant_id=$2`, id, tenant))
}
func (r *Repository) UpdateInvoice(ctx context.Context, v domain.Invoice) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE invoices SET provider_reference=$1,provider_request_date=$2,qr_payload=$3,status=$4,updated_at=$5,last_checked_at=$6,check_count=$7 WHERE id=$8 AND tenant_id=$9`, v.ProviderReference, v.ProviderRequestDate, v.QRPayload, v.Status, v.UpdatedAt, v.LastCheckedAt, v.CheckCount, v.ID, v.TenantID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
func (r *Repository) UpdatePendingInvoice(ctx context.Context, v domain.Invoice) (bool, error) {
	res, err := r.DB.ExecContext(ctx, `UPDATE invoices SET provider_reference=$1,provider_request_date=$2,qr_payload=$3,status=$4,updated_at=$5,last_checked_at=$6,check_count=$7 WHERE id=$8 AND tenant_id=$9 AND status='pending'`, v.ProviderReference, v.ProviderRequestDate, v.QRPayload, v.Status, v.UpdatedAt, v.LastCheckedAt, v.CheckCount, v.ID, v.TenantID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if n == 1 && v.Status != domain.InvoicePending {
		_, err = r.DB.ExecContext(ctx, `UPDATE hosted_invoice_unique_amount_reservations SET state='cooldown',terminal_status=$1,terminal_at=$2,cooldown_until=$2 + (cooldown_minutes * INTERVAL '1 minute') WHERE invoice_id=$3 AND unique_amount_code > 0`, v.Status, v.UpdatedAt, v.ID)
	}
	return n == 1, err
}
func (r *Repository) CancelPendingInvoice(ctx context.Context, v domain.Invoice) (bool, error) {
	res, err := r.DB.ExecContext(ctx, `UPDATE invoices SET status='cancelled',updated_at=$1,last_checked_at=NULL WHERE id=$2 AND tenant_id=$3 AND status='pending'`, v.UpdatedAt, v.ID, v.TenantID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (r *Repository) PendingInvoices(ctx context.Context, due time.Time, limit int) ([]domain.Invoice, error) {
	rows, err := r.DB.QueryContext(ctx, invoiceColumns+` WHERE status='pending' AND (last_checked_at IS NULL OR last_checked_at <= $1) ORDER BY created_at LIMIT $2`, due, limit)
	return scanInvoices(rows, err)
}
func (r *Repository) ListInvoices(ctx context.Context, tenant string, limit int) ([]domain.Invoice, error) {
	q := invoiceColumns
	args := []any{}
	if tenant != "" {
		q += ` WHERE tenant_id=$1`
		args = append(args, tenant)
	}
	args = append(args, limit)
	q += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args))
	rows, err := r.DB.QueryContext(ctx, q, args...)
	return scanInvoices(rows, err)
}
func (r *Repository) CreateQRISTemplate(ctx context.Context, v domain.QRISTemplate) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO qris_templates(id,tenant_id,name,static_payload,image_mime,image_data,merchant_name,merchant_city,access_scope,static_to_dynamic,max_requests_per_minute,active,created_at) VALUES($1,NULLIF($2,''),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, v.ID, v.TenantID, v.Name, v.StaticPayload, v.ImageMIME, v.ImageData, v.MerchantName, v.MerchantCity, v.AccessScope, v.StaticToDynamic, v.MaxRequestsPM, v.Active, v.CreatedAt)
	return err
}
func (r *Repository) UpdateQRISTemplate(ctx context.Context, v domain.QRISTemplate) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE qris_templates SET tenant_id=NULLIF($1,''),name=$2,access_scope=$3,static_to_dynamic=$4,max_requests_per_minute=$5,active=$6 WHERE id=$7`, v.TenantID, v.Name, v.AccessScope, v.StaticToDynamic, v.MaxRequestsPM, v.Active, v.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
func (r *Repository) QRISTemplate(ctx context.Context, id string) (domain.QRISTemplate, error) {
	var v domain.QRISTemplate
	err := r.DB.QueryRowContext(ctx, `SELECT id,COALESCE(tenant_id,''),name,static_payload,image_mime,image_data,merchant_name,merchant_city,access_scope,static_to_dynamic,max_requests_per_minute,active,created_at FROM qris_templates WHERE id=$1`, id).
		Scan(&v.ID, &v.TenantID, &v.Name, &v.StaticPayload, &v.ImageMIME, &v.ImageData, &v.MerchantName, &v.MerchantCity, &v.AccessScope, &v.StaticToDynamic, &v.MaxRequestsPM, &v.Active, &v.CreatedAt)
	return v, notFound(err)
}
func (r *Repository) ListQRISTemplates(ctx context.Context) ([]domain.QRISTemplate, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,COALESCE(tenant_id,''),name,static_payload,image_mime,image_data,merchant_name,merchant_city,access_scope,static_to_dynamic,max_requests_per_minute,active,created_at FROM qris_templates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.QRISTemplate, 0)
	for rows.Next() {
		var v domain.QRISTemplate
		if err := rows.Scan(&v.ID, &v.TenantID, &v.Name, &v.StaticPayload, &v.ImageMIME, &v.ImageData, &v.MerchantName, &v.MerchantCity, &v.AccessScope, &v.StaticToDynamic, &v.MaxRequestsPM, &v.Active, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) AllowQRISRequest(ctx context.Context, templateID, tenantID string, now time.Time, max int) (bool, int, error) {
	windowStart := now.UTC().Truncate(time.Minute)
	var count int
	err := r.DB.QueryRowContext(ctx, `INSERT INTO qris_template_rate_limits(template_id,tenant_id,window_started_at,request_count) VALUES($1,$2,$3,1) ON CONFLICT(template_id,tenant_id) DO UPDATE SET window_started_at=CASE WHEN qris_template_rate_limits.window_started_at < EXCLUDED.window_started_at THEN EXCLUDED.window_started_at ELSE qris_template_rate_limits.window_started_at END,request_count=CASE WHEN qris_template_rate_limits.window_started_at < EXCLUDED.window_started_at THEN 1 ELSE qris_template_rate_limits.request_count+1 END RETURNING request_count`, templateID, tenantID, windowStart).Scan(&count)
	retry := int(windowStart.Add(time.Minute).Sub(now.UTC()).Seconds())
	if retry < 1 {
		retry = 1
	}
	return count <= max, retry, err
}

type uniqueAmountReservation struct {
	PaymentID       string
	TenantID        string
	MerchantID      string
	PayableAmount   int64
	Code            int64
	CooldownMinutes int
	CooldownUntil   time.Time
}

func cleanupUniqueAmountCooldowns(ctx context.Context, tx *sql.Tx, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT payment_id,COALESCE(tenant_id,''),merchant_id,payable_amount,unique_amount_code,cooldown_minutes,cooldown_until FROM qris_unique_amount_reservations WHERE state='cooldown' AND cooldown_until <= $1 FOR UPDATE`, now)
	if err != nil {
		return err
	}
	reservations := make([]uniqueAmountReservation, 0)
	for rows.Next() {
		var reservation uniqueAmountReservation
		if err := rows.Scan(&reservation.PaymentID, &reservation.TenantID, &reservation.MerchantID, &reservation.PayableAmount, &reservation.Code, &reservation.CooldownMinutes, &reservation.CooldownUntil); err != nil {
			rows.Close()
			return err
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, reservation := range reservations {
		metadata := map[string]any{"merchant_id": reservation.MerchantID, "code": reservation.Code, "payable_amount": reservation.PayableAmount, "cooldown_until": reservation.CooldownUntil}
		if err := insertUniqueAmountAudit(ctx, tx, "qris-code-cooldown-ended-"+reservation.PaymentID, reservation.TenantID, "qris.unique_amount.cooldown_ended", reservation.PaymentID, metadata, reservation.CooldownUntil); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM qris_unique_amount_reservations WHERE payment_id=$1 AND state='cooldown'`, reservation.PaymentID); err != nil {
			return err
		}
	}
	return nil
}

func reserveUniqueAmount(ctx context.Context, tx *sql.Tx, payment domain.TestPayment, now time.Time) error {
	if err := cleanupUniqueAmountCooldowns(ctx, tx, now); err != nil {
		return err
	}
	cooldownMinutes := 30
	if payment.TenantID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT unique_amount_cooldown_minutes FROM tenants WHERE id=$1`, payment.TenantID).Scan(&cooldownMinutes); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO qris_unique_amount_reservations(payment_id,tenant_id,merchant_id,payable_amount,expires_at,unique_amount_code,state,reserved_at,cooldown_minutes) VALUES($1,NULLIF($2,''),$3,$4,$5,$6,'active',$7,$8)`, payment.ID, payment.TenantID, payment.MerchantID, payment.PayableAmount, payment.ExpiresAt, payment.UniqueAmountCode, payment.CreatedAt, cooldownMinutes)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "qris_unique_amount_reservations_merchant_payable_key" {
			return store.ErrUniqueAmountUnavailable
		}
		return err
	}
	if payment.UniqueAmountCode == 0 {
		return nil
	}
	metadata := map[string]any{"merchant_id": payment.MerchantID, "code": payment.UniqueAmountCode, "payable_amount": payment.PayableAmount, "reserved_at": payment.CreatedAt, "cooldown_minutes": cooldownMinutes}
	return insertUniqueAmountAudit(ctx, tx, "qris-code-reserved-"+payment.ID, payment.TenantID, "qris.unique_amount.reserved", payment.ID, metadata, payment.CreatedAt)
}

func startUniqueAmountCooldown(ctx context.Context, tx *sql.Tx, payment domain.TestPayment) error {
	var reservation uniqueAmountReservation
	err := tx.QueryRowContext(ctx, `SELECT payment_id,COALESCE(tenant_id,''),merchant_id,payable_amount,unique_amount_code,cooldown_minutes FROM qris_unique_amount_reservations WHERE payment_id=$1 FOR UPDATE`, payment.ID).Scan(&reservation.PaymentID, &reservation.TenantID, &reservation.MerchantID, &reservation.PayableAmount, &reservation.Code, &reservation.CooldownMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if (reservation.Code == 0 && payment.Status != domain.InvoiceCancelled) || (payment.Status != domain.InvoicePaid && payment.Status != domain.InvoiceExpired && payment.Status != domain.InvoiceCancelled) {
		_, err = tx.ExecContext(ctx, `DELETE FROM qris_unique_amount_reservations WHERE payment_id=$1`, payment.ID)
		return err
	}
	reservation.CooldownUntil = payment.UpdatedAt.Add(time.Duration(reservation.CooldownMinutes) * time.Minute)
	if _, err = tx.ExecContext(ctx, `UPDATE qris_unique_amount_reservations SET state='cooldown',terminal_status=$1,terminal_at=$2,cooldown_until=$3 WHERE payment_id=$4`, payment.Status, payment.UpdatedAt, reservation.CooldownUntil, payment.ID); err != nil {
		return err
	}
	metadata := map[string]any{"merchant_id": reservation.MerchantID, "code": reservation.Code, "payable_amount": reservation.PayableAmount, "terminal_status": payment.Status, "terminal_at": payment.UpdatedAt, "cooldown_until": reservation.CooldownUntil}
	return insertUniqueAmountAudit(ctx, tx, "qris-code-cooldown-"+payment.ID, reservation.TenantID, "qris.unique_amount.cooldown_started", payment.ID, metadata, payment.UpdatedAt)
}

func insertUniqueAmountAudit(ctx context.Context, tx *sql.Tx, id, tenantID, action, paymentID string, metadata map[string]any, createdAt time.Time) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor,action,resource_type,resource_id,metadata,created_at) VALUES($1,NULLIF($2,''),'system',$3,'unique_amount_code',$4,$5,$6) ON CONFLICT(id) DO NOTHING`, id, tenantID, action, paymentID, raw, createdAt)
	return err
}

func (r *Repository) CreateTestPayment(ctx context.Context, v domain.TestPayment) error {
	if v.PayableAmount <= 0 {
		v.PayableAmount = v.Amount
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO test_payments(id,idempotency_key,qris_template_id,merchant_id,tenant_id,amount,payable_amount,unique_amount_code,dynamic_payload,unique_code,status,request_source,match_confidence,matched_transaction_id,created_at,updated_at,expires_at,last_checked_at,next_check_at,check_count,sandbox_mode) VALUES($1,NULLIF($2,''),$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),$15,$16,$17,$18,$19,$20,$21)`, v.ID, v.IdempotencyKey, v.QRISTemplateID, v.MerchantID, v.TenantID, v.Amount, v.PayableAmount, v.UniqueAmountCode, v.DynamicPayload, v.UniqueCode, v.Status, v.RequestSource, v.MatchConfidence, v.MatchedTransactionID, v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.LastCheckedAt, v.NextCheckAt, v.CheckCount, v.SandboxMode); err != nil {
		return err
	}
	if v.Status == domain.InvoicePending && v.MerchantID != "" && (v.RequestSource == "tenant_api" || v.RequestSource == "admin_qris_test") {
		var overlap bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM test_payments WHERE id<>$1 AND merchant_id=$2 AND payable_amount=$3 AND status='pending' AND expires_at>$4)`, v.ID, v.MerchantID, v.PayableAmount, v.CreatedAt).Scan(&overlap); err != nil {
			return err
		}
		if overlap {
			return store.ErrUniqueAmountUnavailable
		}
		if err = reserveUniqueAmount(ctx, tx, v, v.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) CreateTenantTestPayment(ctx context.Context, v domain.TestPayment, now time.Time, max int) (domain.TestPayment, bool, bool, int, error) {
	if v.PayableAmount <= 0 {
		v.PayableAmount = v.Amount
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.TestPayment{}, false, false, 0, err
	}
	defer tx.Rollback()
	existing, lookupErr := scanTestPayment(tx.QueryRowContext(ctx, testPaymentColumns+` WHERE tenant_id=$1 AND idempotency_key=$2 AND request_source='tenant_api'`, v.TenantID, v.IdempotencyKey))
	if lookupErr == nil {
		if existing.Amount != v.Amount || existing.QRISTemplateID != v.QRISTemplateID || existing.ExpiresAt.Sub(existing.CreatedAt) != v.ExpiresAt.Sub(v.CreatedAt) {
			return domain.TestPayment{}, false, false, 0, store.ErrConflict
		}
		return existing, false, true, 0, nil
	}
	if !errors.Is(lookupErr, sql.ErrNoRows) {
		return domain.TestPayment{}, false, false, 0, lookupErr
	}
	windowStart := now.UTC().Truncate(time.Minute)
	var count int
	err = tx.QueryRowContext(ctx, `INSERT INTO qris_template_rate_limits(template_id,tenant_id,window_started_at,request_count) VALUES($1,$2,$3,1) ON CONFLICT(template_id,tenant_id) DO UPDATE SET window_started_at=CASE WHEN qris_template_rate_limits.window_started_at < EXCLUDED.window_started_at THEN EXCLUDED.window_started_at ELSE qris_template_rate_limits.window_started_at END,request_count=CASE WHEN qris_template_rate_limits.window_started_at < EXCLUDED.window_started_at THEN 1 ELSE qris_template_rate_limits.request_count+1 END RETURNING request_count`, v.QRISTemplateID, v.TenantID, windowStart).Scan(&count)
	if err != nil {
		return domain.TestPayment{}, false, false, 0, err
	}
	existing, lookupErr = scanTestPayment(tx.QueryRowContext(ctx, testPaymentColumns+` WHERE tenant_id=$1 AND idempotency_key=$2 AND request_source='tenant_api'`, v.TenantID, v.IdempotencyKey))
	if lookupErr == nil {
		if existing.Amount != v.Amount || existing.QRISTemplateID != v.QRISTemplateID || existing.ExpiresAt.Sub(existing.CreatedAt) != v.ExpiresAt.Sub(v.CreatedAt) {
			return domain.TestPayment{}, false, false, 0, store.ErrConflict
		}
		return existing, false, true, 0, nil
	}
	if !errors.Is(lookupErr, sql.ErrNoRows) {
		return domain.TestPayment{}, false, false, 0, lookupErr
	}
	retry := int(windowStart.Add(time.Minute).Sub(now.UTC()).Seconds())
	if retry < 1 {
		retry = 1
	}
	if count > max {
		return domain.TestPayment{}, false, false, retry, nil
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO test_payments(id,idempotency_key,qris_template_id,merchant_id,tenant_id,amount,payable_amount,unique_amount_code,dynamic_payload,unique_code,status,request_source,match_confidence,matched_transaction_id,created_at,updated_at,expires_at,last_checked_at,next_check_at,check_count,sandbox_mode) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,'tenant_api',$12,NULLIF($13,''),$14,$15,$16,$17,$18,$19,$20) ON CONFLICT(tenant_id,idempotency_key) WHERE request_source='tenant_api' AND idempotency_key IS NOT NULL DO NOTHING`, v.ID, v.IdempotencyKey, v.QRISTemplateID, v.MerchantID, v.TenantID, v.Amount, v.PayableAmount, v.UniqueAmountCode, v.DynamicPayload, v.UniqueCode, v.Status, v.MatchConfidence, v.MatchedTransactionID, v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.LastCheckedAt, v.NextCheckAt, v.CheckCount, v.SandboxMode)
	if err != nil {
		return domain.TestPayment{}, false, false, 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return domain.TestPayment{}, false, false, 0, err
	}
	if n == 1 {
		if err = cleanupUniqueAmountCooldowns(ctx, tx, now); err != nil {
			return domain.TestPayment{}, false, false, 0, err
		}
		var legacyOverlap bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM test_payments WHERE id<>$1 AND merchant_id=$2 AND payable_amount=$3 AND status='pending' AND expires_at>$4)`, v.ID, v.MerchantID, v.PayableAmount, now).Scan(&legacyOverlap); err != nil {
			return domain.TestPayment{}, false, false, 0, err
		}
		if legacyOverlap {
			return domain.TestPayment{}, false, false, 0, store.ErrUniqueAmountUnavailable
		}
		if err = reserveUniqueAmount(ctx, tx, v, now); err != nil {
			return domain.TestPayment{}, false, false, 0, err
		}
	}
	stored, err := scanTestPayment(tx.QueryRowContext(ctx, testPaymentColumns+` WHERE tenant_id=$1 AND idempotency_key=$2 AND request_source='tenant_api'`, v.TenantID, v.IdempotencyKey))
	if err != nil {
		return domain.TestPayment{}, false, false, 0, err
	}
	if stored.Amount != v.Amount || stored.QRISTemplateID != v.QRISTemplateID || stored.ExpiresAt.Sub(stored.CreatedAt) != v.ExpiresAt.Sub(v.CreatedAt) {
		return domain.TestPayment{}, false, false, 0, store.ErrConflict
	}
	if err = tx.Commit(); err != nil {
		return domain.TestPayment{}, false, false, 0, err
	}
	return stored, n == 1, true, 0, nil
}

const testPaymentColumns = `SELECT id,COALESCE(idempotency_key,''),qris_template_id,COALESCE(merchant_id,''),COALESCE(tenant_id,''),amount,payable_amount,unique_amount_code,dynamic_payload,unique_code,status,request_source,match_confidence,COALESCE(matched_transaction_id,''),created_at,updated_at,expires_at,last_checked_at,next_check_at,check_count,sandbox_mode FROM test_payments`

func scanTestPayment(s interface{ Scan(...any) error }) (domain.TestPayment, error) {
	var v domain.TestPayment
	err := s.Scan(&v.ID, &v.IdempotencyKey, &v.QRISTemplateID, &v.MerchantID, &v.TenantID, &v.Amount, &v.PayableAmount, &v.UniqueAmountCode, &v.DynamicPayload, &v.UniqueCode, &v.Status, &v.RequestSource, &v.MatchConfidence, &v.MatchedTransactionID, &v.CreatedAt, &v.UpdatedAt, &v.ExpiresAt, &v.LastCheckedAt, &v.NextCheckAt, &v.CheckCount, &v.SandboxMode)
	return v, err
}
func (r *Repository) TestPayment(ctx context.Context, id string) (domain.TestPayment, error) {
	v, err := scanTestPayment(r.DB.QueryRowContext(ctx, testPaymentColumns+` WHERE id=$1`, id))
	return v, notFound(err)
}
func (r *Repository) TestPaymentForTenant(ctx context.Context, tenantID, id string) (domain.TestPayment, error) {
	v, err := scanTestPayment(r.DB.QueryRowContext(ctx, testPaymentColumns+` WHERE tenant_id=$1 AND id=$2 AND request_source='tenant_api'`, tenantID, id))
	return v, notFound(err)
}
func (r *Repository) CancelPendingTestPayment(ctx context.Context, tenantID, id string, now time.Time, audit domain.AuditEvent) (domain.TestPayment, bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.TestPayment{}, false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE test_payments SET status='cancelled',match_confidence='cancelled_by_tenant',updated_at=$1,next_check_at=NULL WHERE id=$2 AND tenant_id=$3 AND request_source='tenant_api' AND status='pending' AND expires_at>$1`, now, id, tenantID)
	if err != nil {
		return domain.TestPayment{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return domain.TestPayment{}, false, err
	}
	expired := false
	if n == 0 {
		expireRes, expireErr := tx.ExecContext(ctx, `UPDATE test_payments SET status='expired',match_confidence='expired_no_match',updated_at=$1,next_check_at=NULL WHERE id=$2 AND tenant_id=$3 AND request_source='tenant_api' AND status='pending' AND expires_at<=$1`, now, id, tenantID)
		if expireErr != nil {
			return domain.TestPayment{}, false, expireErr
		}
		expiredRows, rowsErr := expireRes.RowsAffected()
		if rowsErr != nil {
			return domain.TestPayment{}, false, rowsErr
		}
		expired = expiredRows == 1
	}
	v, err := scanTestPayment(tx.QueryRowContext(ctx, testPaymentColumns+` WHERE tenant_id=$1 AND id=$2 AND request_source='tenant_api'`, tenantID, id))
	if err != nil {
		return domain.TestPayment{}, false, notFound(err)
	}
	if n == 0 {
		if expired {
			if err = startUniqueAmountCooldown(ctx, tx, v); err != nil {
				return domain.TestPayment{}, false, err
			}
		}
		if err = tx.Commit(); err != nil {
			return domain.TestPayment{}, false, err
		}
		if v.Status == domain.InvoiceCancelled {
			return v, false, nil
		}
		return v, false, store.ErrConflict
	}
	if err = startUniqueAmountCooldown(ctx, tx, v); err != nil {
		return domain.TestPayment{}, false, err
	}
	metadata, err := json.Marshal(audit.Metadata)
	if err != nil {
		return domain.TestPayment{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor,action,resource_type,resource_id,metadata,created_at) VALUES($1,NULLIF($2,''),$3,$4,$5,$6,$7,$8)`, audit.ID, audit.TenantID, audit.Actor, audit.Action, audit.ResourceType, audit.ResourceID, metadata, audit.CreatedAt); err != nil {
		return domain.TestPayment{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return domain.TestPayment{}, false, err
	}
	return v, true, nil
}
func (r *Repository) UpdatePendingTestPayment(ctx context.Context, v domain.TestPayment) (bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE test_payments SET status=$1,match_confidence=$2,matched_transaction_id=NULLIF($3,''),updated_at=$4,last_checked_at=$5,next_check_at=$6,check_count=$7 WHERE id=$8 AND status='pending' AND check_count=$9`, v.Status, v.MatchConfidence, v.MatchedTransactionID, v.UpdatedAt, v.LastCheckedAt, v.NextCheckAt, v.CheckCount, v.ID, v.CheckCount-1)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return false, nil
	}
	if v.Status != domain.InvoicePending {
		if err = startUniqueAmountCooldown(ctx, tx, v); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func (r *Repository) MatchPendingTestPayment(ctx context.Context, v domain.TestPayment, transaction domain.PortalTransaction) (bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE test_payments SET status=$1,match_confidence=$2,matched_transaction_id=$3,updated_at=$4,last_checked_at=$5,next_check_at=$6,check_count=$7 WHERE id=$8 AND status='pending' AND check_count=$9`, v.Status, v.MatchConfidence, v.MatchedTransactionID, v.UpdatedAt, v.LastCheckedAt, v.NextCheckAt, v.CheckCount, v.ID, v.CheckCount-1)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "test_payments_matched_transaction_unique" {
			return false, nil
		}
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO portal_transactions(id,merchant_id,tenant_id,reference,amount,status,paid_at,source,match_confidence,invoice_id,created_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11) ON CONFLICT(merchant_id,reference) DO UPDATE SET amount=EXCLUDED.amount,status=EXCLUDED.status,paid_at=EXCLUDED.paid_at,source=EXCLUDED.source,tenant_id=COALESCE(portal_transactions.tenant_id,EXCLUDED.tenant_id),invoice_id=COALESCE(portal_transactions.invoice_id,EXCLUDED.invoice_id),match_confidence=CASE WHEN portal_transactions.tenant_id IS NULL THEN EXCLUDED.match_confidence ELSE portal_transactions.match_confidence END`, transaction.ID, transaction.MerchantID, transaction.TenantID, transaction.Reference, transaction.Amount, transaction.Status, transaction.PaidAt, transaction.Source, transaction.MatchConfidence, transaction.InvoiceID, transaction.CreatedAt)
	if err != nil {
		return false, err
	}
	if err = startUniqueAmountCooldown(ctx, tx, v); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
func (r *Repository) PendingTestPayments(ctx context.Context, due time.Time, limit int) ([]domain.TestPayment, error) {
	rows, err := r.DB.QueryContext(ctx, testPaymentColumns+` WHERE status='pending' AND next_check_at IS NOT NULL AND next_check_at <= $1 ORDER BY next_check_at,created_at LIMIT $2`, due, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TestPayment, 0)
	for rows.Next() {
		v, scanErr := scanTestPayment(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) PendingConnectedTestPayments(ctx context.Context, due time.Time, limit int) ([]domain.TestPayment, error) {
	rows, err := r.DB.QueryContext(ctx, testPaymentColumns+` WHERE test_payments.status='pending' AND test_payments.next_check_at IS NOT NULL AND test_payments.next_check_at <= $1 AND EXISTS (SELECT 1 FROM merchant_connections WHERE merchant_connections.merchant_id=test_payments.merchant_id AND merchant_connections.status='connected') ORDER BY test_payments.next_check_at,test_payments.created_at,test_payments.id LIMIT $2`, due, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TestPayment, 0)
	for rows.Next() {
		v, scanErr := scanTestPayment(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) ExpirePendingTestPayments(ctx context.Context, now time.Time) (int64, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `UPDATE test_payments SET status='expired',match_confidence='expired_no_match',updated_at=$1,next_check_at=NULL WHERE status='pending' AND expires_at <= $1 RETURNING id,COALESCE(tenant_id,''),COALESCE(merchant_id,''),amount,payable_amount,unique_amount_code,status,updated_at`, now)
	if err != nil {
		return 0, err
	}
	expired := make([]domain.TestPayment, 0)
	for rows.Next() {
		var payment domain.TestPayment
		if err := rows.Scan(&payment.ID, &payment.TenantID, &payment.MerchantID, &payment.Amount, &payment.PayableAmount, &payment.UniqueAmountCode, &payment.Status, &payment.UpdatedAt); err != nil {
			rows.Close()
			return 0, err
		}
		expired = append(expired, payment)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, payment := range expired {
		if err := startUniqueAmountCooldown(ctx, tx, payment); err != nil {
			return 0, err
		}
	}
	if err := cleanupUniqueAmountCooldowns(ctx, tx, now); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(expired)), nil
}
func (r *Repository) ListTestPayments(ctx context.Context, limit int) ([]domain.TestPayment, error) {
	rows, err := r.DB.QueryContext(ctx, testPaymentColumns+` ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TestPayment, 0)
	for rows.Next() {
		v, scanErr := scanTestPayment(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) ListTenantTestPayments(ctx context.Context, tenantID string, limit int) ([]domain.TestPayment, error) {
	query := testPaymentColumns + ` WHERE request_source='tenant_api'`
	args := []any{}
	if tenantID != "" {
		query += ` AND tenant_id=$1`
		args = append(args, tenantID)
	}
	args = append(args, limit)
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args))
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TestPayment, 0)
	for rows.Next() {
		v, scanErr := scanTestPayment(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) AppendAudit(ctx context.Context, v domain.AuditEvent) error {
	meta, _ := json.Marshal(v.Metadata)
	_, err := r.DB.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor,action,resource_type,resource_id,metadata,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, null(v.TenantID), v.Actor, v.Action, v.ResourceType, v.ResourceID, meta, v.CreatedAt)
	return err
}
func (r *Repository) ListAudit(ctx context.Context, tenant string, limit int) ([]domain.AuditEvent, error) {
	q := `SELECT id,COALESCE(tenant_id,''),actor,action,resource_type,resource_id,metadata,created_at FROM audit_events`
	args := []any{}
	if tenant != "" {
		q += ` WHERE tenant_id=$1`
		args = append(args, tenant)
	}
	args = append(args, limit)
	q += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args))
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var v domain.AuditEvent
		var raw []byte
		if err := rows.Scan(&v.ID, &v.TenantID, &v.Actor, &v.Action, &v.ResourceType, &v.ResourceID, &raw, &v.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &v.Metadata)
		out = append(out, v)
	}
	return out, rows.Err()
}

const invoiceColumns = `SELECT id,tenant_id,merchant_account_id,idempotency_key,COALESCE(requested_amount,amount),amount,unique_amount_code,COALESCE(qris_template_id,''),COALESCE(qris_merchant_id,''),COALESCE(qris_merchant_name,''),COALESCE(qris_merchant_city,''),currency,description,provider_reference,provider_request_date,qr_payload,status,created_at,updated_at,expires_at,last_checked_at,check_count,sandbox_mode FROM invoices`

type scanner interface{ Scan(...any) error }

func scanInvoice(s scanner) (domain.Invoice, error) {
	var v domain.Invoice
	err := s.Scan(&v.ID, &v.TenantID, &v.MerchantAccountID, &v.IdempotencyKey, &v.RequestedAmount, &v.Amount, &v.UniqueAmountCode, &v.QRISTemplateID, &v.QRISMerchantID, &v.QRISMerchantName, &v.QRISMerchantCity, &v.Currency, &v.Description, &v.ProviderReference, &v.ProviderRequestDate, &v.QRPayload, &v.Status, &v.CreatedAt, &v.UpdatedAt, &v.ExpiresAt, &v.LastCheckedAt, &v.CheckCount, &v.SandboxMode)
	return v, notFound(err)
}
func scanInvoices(rows *sql.Rows, err error) ([]domain.Invoice, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Invoice, 0)
	for rows.Next() {
		v, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	return err
}
func null(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func normalizedCooldownMinutes(minutes int) int {
	if minutes < 30 || minutes > 60 {
		return 30
	}
	return minutes
}
