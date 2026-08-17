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
	err := r.DB.QueryRowContext(ctx, `SELECT id,COALESCE(merchant_id,''),name,COALESCE(site_url,''),COALESCE(callback_url,''),COALESCE(webhook_url,''),sandbox_mode,use_unique_amount_code,api_key_hash,COALESCE(api_key_ciphertext,''),active,created_at FROM tenants WHERE api_key_hash=$1 AND active=true`, h).Scan(&v.ID, &v.MerchantID, &v.Name, &v.SiteURL, &v.CallbackURL, &v.WebhookURL, &v.SandboxMode, &v.UseUniqueAmountCode, &v.APIKeyHash, &v.APIKeyCiphertext, &v.Active, &v.CreatedAt)
	v.APIKeyRecoverable = v.APIKeyCiphertext != ""
	return v, notFound(err)
}
func (r *Repository) Tenant(ctx context.Context, id string) (domain.Tenant, error) {
	var v domain.Tenant
	err := r.DB.QueryRowContext(ctx, `SELECT id,COALESCE(merchant_id,''),name,COALESCE(site_url,''),COALESCE(callback_url,''),COALESCE(webhook_url,''),sandbox_mode,use_unique_amount_code,api_key_hash,COALESCE(api_key_ciphertext,''),active,created_at FROM tenants WHERE id=$1`, id).Scan(&v.ID, &v.MerchantID, &v.Name, &v.SiteURL, &v.CallbackURL, &v.WebhookURL, &v.SandboxMode, &v.UseUniqueAmountCode, &v.APIKeyHash, &v.APIKeyCiphertext, &v.Active, &v.CreatedAt)
	v.APIKeyRecoverable = v.APIKeyCiphertext != ""
	return v, notFound(err)
}
func (r *Repository) CreateTenant(ctx context.Context, v domain.Tenant) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO tenants(id,merchant_id,name,site_url,callback_url,webhook_url,sandbox_mode,use_unique_amount_code,api_key_hash,api_key_ciphertext,active,created_at) VALUES($1,NULLIF($2,''),$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),$7,$8,$9,NULLIF($10,''),$11,$12)`, v.ID, v.MerchantID, v.Name, v.SiteURL, v.CallbackURL, v.WebhookURL, v.SandboxMode, v.UseUniqueAmountCode, v.APIKeyHash, v.APIKeyCiphertext, v.Active, v.CreatedAt)
	return err
}
func (r *Repository) UpdateTenant(ctx context.Context, v domain.Tenant) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE tenants SET merchant_id=NULLIF($1,''),name=$2,site_url=NULLIF($3,''),callback_url=NULLIF($4,''),webhook_url=NULLIF($5,''),sandbox_mode=$6,use_unique_amount_code=$7,active=$8 WHERE id=$9`, v.MerchantID, v.Name, v.SiteURL, v.CallbackURL, v.WebhookURL, v.SandboxMode, v.UseUniqueAmountCode, v.Active, v.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
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
func (r *Repository) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,COALESCE(merchant_id,''),name,COALESCE(site_url,''),COALESCE(callback_url,''),COALESCE(webhook_url,''),sandbox_mode,use_unique_amount_code,api_key_hash,COALESCE(api_key_ciphertext,''),active,created_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Tenant, 0)
	for rows.Next() {
		var v domain.Tenant
		if err := rows.Scan(&v.ID, &v.MerchantID, &v.Name, &v.SiteURL, &v.CallbackURL, &v.WebhookURL, &v.SandboxMode, &v.UseUniqueAmountCode, &v.APIKeyHash, &v.APIKeyCiphertext, &v.Active, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.APIKeyRecoverable = v.APIKeyCiphertext != ""
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
	res, err := r.DB.ExecContext(ctx, `INSERT INTO invoices(id,tenant_id,merchant_account_id,idempotency_key,amount,currency,description,provider_reference,provider_request_date,qr_payload,status,created_at,updated_at,expires_at,check_count,sandbox_mode) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT(tenant_id,idempotency_key) DO NOTHING`, v.ID, v.TenantID, v.MerchantAccountID, v.IdempotencyKey, v.Amount, v.Currency, v.Description, v.ProviderReference, v.ProviderRequestDate, v.QRPayload, v.Status, v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.CheckCount, v.SandboxMode)
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
		if _, err = tx.ExecContext(ctx, `DELETE FROM qris_unique_amount_reservations WHERE expires_at <= $1`, v.CreatedAt); err != nil {
			return err
		}
		var overlap bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM test_payments WHERE id<>$1 AND merchant_id=$2 AND payable_amount=$3 AND status='pending' AND expires_at>$4)`, v.ID, v.MerchantID, v.PayableAmount, v.CreatedAt).Scan(&overlap); err != nil {
			return err
		}
		if overlap {
			return store.ErrUniqueAmountUnavailable
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO qris_unique_amount_reservations(payment_id,merchant_id,payable_amount,expires_at) VALUES($1,$2,$3,$4)`, v.ID, v.MerchantID, v.PayableAmount, v.ExpiresAt); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "qris_unique_amount_reservations_merchant_payable_key" {
				return store.ErrUniqueAmountUnavailable
			}
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
		if _, err = tx.ExecContext(ctx, `DELETE FROM qris_unique_amount_reservations WHERE expires_at <= $1`, now); err != nil {
			return domain.TestPayment{}, false, false, 0, err
		}
		var legacyOverlap bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM test_payments WHERE id<>$1 AND merchant_id=$2 AND payable_amount=$3 AND status='pending' AND expires_at>$4)`, v.ID, v.MerchantID, v.PayableAmount, now).Scan(&legacyOverlap); err != nil {
			return domain.TestPayment{}, false, false, 0, err
		}
		if legacyOverlap {
			return domain.TestPayment{}, false, false, 0, store.ErrUniqueAmountUnavailable
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO qris_unique_amount_reservations(payment_id,merchant_id,payable_amount,expires_at) VALUES($1,$2,$3,$4)`, v.ID, v.MerchantID, v.PayableAmount, v.ExpiresAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "qris_unique_amount_reservations_merchant_payable_key" {
				return domain.TestPayment{}, false, false, 0, store.ErrUniqueAmountUnavailable
			}
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
		if _, err = tx.ExecContext(ctx, `DELETE FROM qris_unique_amount_reservations WHERE payment_id=$1`, v.ID); err != nil {
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
	if _, err = tx.ExecContext(ctx, `DELETE FROM qris_unique_amount_reservations WHERE payment_id=$1`, v.ID); err != nil {
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
func (r *Repository) ExpirePendingTestPayments(ctx context.Context, now time.Time) (int64, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE test_payments SET status='expired',match_confidence='expired_no_match',updated_at=$1,next_check_at=NULL WHERE status='pending' AND expires_at <= $1`, now)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM qris_unique_amount_reservations WHERE expires_at <= $1`, now); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
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

const invoiceColumns = `SELECT id,tenant_id,merchant_account_id,idempotency_key,amount,currency,description,provider_reference,provider_request_date,qr_payload,status,created_at,updated_at,expires_at,last_checked_at,check_count,sandbox_mode FROM invoices`

type scanner interface{ Scan(...any) error }

func scanInvoice(s scanner) (domain.Invoice, error) {
	var v domain.Invoice
	err := s.Scan(&v.ID, &v.TenantID, &v.MerchantAccountID, &v.IdempotencyKey, &v.Amount, &v.Currency, &v.Description, &v.ProviderReference, &v.ProviderRequestDate, &v.QRPayload, &v.Status, &v.CreatedAt, &v.UpdatedAt, &v.ExpiresAt, &v.LastCheckedAt, &v.CheckCount, &v.SandboxMode)
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
