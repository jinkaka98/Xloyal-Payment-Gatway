package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

type Repository struct{ DB *sql.DB }

var _ store.Repository = (*Repository)(nil)

func New(db *sql.DB) *Repository                       { return &Repository{DB: db} }
func (r *Repository) Health(ctx context.Context) error { return r.DB.PingContext(ctx) }
func (r *Repository) TenantByAPIKey(ctx context.Context, h string) (domain.Tenant, error) {
	var v domain.Tenant
	err := r.DB.QueryRowContext(ctx, `SELECT id,name,api_key_hash,active,created_at FROM tenants WHERE api_key_hash=$1 AND active=true`, h).Scan(&v.ID, &v.Name, &v.APIKeyHash, &v.Active, &v.CreatedAt)
	return v, notFound(err)
}
func (r *Repository) CreateTenant(ctx context.Context, v domain.Tenant) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO tenants(id,name,api_key_hash,active,created_at) VALUES($1,$2,$3,$4,$5)`, v.ID, v.Name, v.APIKeyHash, v.Active, v.CreatedAt)
	return err
}
func (r *Repository) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,name,api_key_hash,active,created_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Tenant, 0)
	for rows.Next() {
		var v domain.Tenant
		if err := rows.Scan(&v.ID, &v.Name, &v.APIKeyHash, &v.Active, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
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
	res, err := r.DB.ExecContext(ctx, `INSERT INTO invoices(id,tenant_id,merchant_account_id,idempotency_key,amount,currency,description,provider_reference,provider_request_date,qr_payload,status,created_at,updated_at,expires_at,check_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT(tenant_id,idempotency_key) DO NOTHING`, v.ID, v.TenantID, v.MerchantAccountID, v.IdempotencyKey, v.Amount, v.Currency, v.Description, v.ProviderReference, v.ProviderRequestDate, v.QRPayload, v.Status, v.CreatedAt, v.UpdatedAt, v.ExpiresAt, v.CheckCount)
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
	_, err := r.DB.ExecContext(ctx, `INSERT INTO qris_templates(id,name,static_payload,image_mime,image_data,merchant_name,merchant_city,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.Name, v.StaticPayload, v.ImageMIME, v.ImageData, v.MerchantName, v.MerchantCity, v.CreatedAt)
	return err
}
func (r *Repository) QRISTemplate(ctx context.Context, id string) (domain.QRISTemplate, error) {
	var v domain.QRISTemplate
	err := r.DB.QueryRowContext(ctx, `SELECT id,name,static_payload,image_mime,image_data,merchant_name,merchant_city,created_at FROM qris_templates WHERE id=$1`, id).
		Scan(&v.ID, &v.Name, &v.StaticPayload, &v.ImageMIME, &v.ImageData, &v.MerchantName, &v.MerchantCity, &v.CreatedAt)
	return v, notFound(err)
}
func (r *Repository) ListQRISTemplates(ctx context.Context) ([]domain.QRISTemplate, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,name,static_payload,image_mime,image_data,merchant_name,merchant_city,created_at FROM qris_templates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.QRISTemplate, 0)
	for rows.Next() {
		var v domain.QRISTemplate
		if err := rows.Scan(&v.ID, &v.Name, &v.StaticPayload, &v.ImageMIME, &v.ImageData, &v.MerchantName, &v.MerchantCity, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) CreateTestPayment(ctx context.Context, v domain.TestPayment) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO test_payments(id,qris_template_id,amount,dynamic_payload,status,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, v.ID, v.QRISTemplateID, v.Amount, v.DynamicPayload, v.Status, v.CreatedAt, v.ExpiresAt)
	return err
}
func (r *Repository) ListTestPayments(ctx context.Context, limit int) ([]domain.TestPayment, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,qris_template_id,amount,dynamic_payload,status,created_at,expires_at FROM test_payments ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TestPayment, 0)
	for rows.Next() {
		var v domain.TestPayment
		if err := rows.Scan(&v.ID, &v.QRISTemplateID, &v.Amount, &v.DynamicPayload, &v.Status, &v.CreatedAt, &v.ExpiresAt); err != nil {
			return nil, err
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

const invoiceColumns = `SELECT id,tenant_id,merchant_account_id,idempotency_key,amount,currency,description,provider_reference,provider_request_date,qr_payload,status,created_at,updated_at,expires_at,last_checked_at,check_count FROM invoices`

type scanner interface{ Scan(...any) error }

func scanInvoice(s scanner) (domain.Invoice, error) {
	var v domain.Invoice
	err := s.Scan(&v.ID, &v.TenantID, &v.MerchantAccountID, &v.IdempotencyKey, &v.Amount, &v.Currency, &v.Description, &v.ProviderReference, &v.ProviderRequestDate, &v.QRPayload, &v.Status, &v.CreatedAt, &v.UpdatedAt, &v.ExpiresAt, &v.LastCheckedAt, &v.CheckCount)
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
