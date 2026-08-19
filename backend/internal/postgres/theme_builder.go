package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"xloyal/backend/internal/domain"
	"xloyal/backend/internal/store"
)

const themeCols = `id,COALESCE(tenant_id,''),name,status,is_default,current_version,COALESCE(draft_config,'{}'::jsonb),created_at,updated_at`

func scanTheme(s interface{ Scan(...any) error }) (domain.PaymentTheme, error) {
	var v domain.PaymentTheme
	err := s.Scan(&v.ID, &v.TenantID, &v.Name, &v.Status, &v.IsDefault, &v.CurrentVersion, &v.DraftConfig, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}
func (r *Repository) ListPaymentThemes(ctx context.Context, tenant string) ([]domain.PaymentTheme, error) {
	var query string
	var args []any
	if tenant == "" {
		query = `SELECT ` + themeCols + ` FROM payment_themes WHERE tenant_id IS NULL ORDER BY updated_at DESC`
	} else {
		query = `SELECT ` + themeCols + ` FROM payment_themes WHERE tenant_id=$1 OR tenant_id IS NULL ORDER BY tenant_id NULLS FIRST,updated_at DESC`
		args = []any{tenant}
	}
	rows, e := r.DB.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.PaymentTheme{}
	for rows.Next() {
		v, e := scanTheme(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) PaymentTheme(ctx context.Context, tenant, id string) (domain.PaymentTheme, error) {
	query := `SELECT ` + themeCols + ` FROM payment_themes WHERE id=$1 AND tenant_id IS NULL`
	args := []any{id}
	if tenant != "" {
		query = `SELECT ` + themeCols + ` FROM payment_themes WHERE id=$1 AND (tenant_id=$2 OR tenant_id IS NULL)`
		args = []any{id, tenant}
	}
	v, e := scanTheme(r.DB.QueryRowContext(ctx, query, args...))
	return v, notFound(e)
}
func (r *Repository) CreatePaymentTheme(ctx context.Context, v domain.PaymentTheme) error {
	_, e := r.DB.ExecContext(ctx, `INSERT INTO payment_themes(id,tenant_id,name,status,is_default,current_version,draft_config,created_at,updated_at)VALUES($1,NULLIF($2,''),$3,'DRAFT',false,0,$4,$5,$5)`, v.ID, v.TenantID, v.Name, v.DraftConfig, v.CreatedAt)
	return e
}
func (r *Repository) UpdatePaymentThemeDraft(ctx context.Context, tenant, id, name string, cfg []byte, now time.Time) (domain.PaymentTheme, error) {
	query := `UPDATE payment_themes SET name=$1,draft_config=$2,status=CASE WHEN status='ARCHIVED' THEN 'DRAFT' ELSE status END,updated_at=$3 WHERE id=$4 AND tenant_id IS NULL RETURNING ` + themeCols
	args := []any{name, cfg, now, id}
	if tenant != "" {
		query = `UPDATE payment_themes SET name=$1,draft_config=$2,status=CASE WHEN status='ARCHIVED' THEN 'DRAFT' ELSE status END,updated_at=$3 WHERE id=$4 AND tenant_id=$5 RETURNING ` + themeCols
		args = []any{name, cfg, now, id, tenant}
	}
	v, e := scanTheme(r.DB.QueryRowContext(ctx, query, args...))
	return v, notFound(e)
}
func (r *Repository) PublishPaymentTheme(ctx context.Context, tenant, id string, now time.Time) (domain.PaymentTheme, domain.PaymentThemeVersion, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return domain.PaymentTheme{}, domain.PaymentThemeVersion{}, e
	}
	defer tx.Rollback()
	query := `SELECT ` + themeCols + ` FROM payment_themes WHERE id=$1 AND tenant_id IS NULL FOR UPDATE`
	args := []any{id}
	if tenant != "" {
		query = `SELECT ` + themeCols + ` FROM payment_themes WHERE id=$1 AND tenant_id=$2 FOR UPDATE`
		args = []any{id, tenant}
	}
	v, e := scanTheme(tx.QueryRowContext(ctx, query, args...))
	if e != nil {
		return v, domain.PaymentThemeVersion{}, notFound(e)
	}
	if len(v.DraftConfig) == 0 {
		return v, domain.PaymentThemeVersion{}, store.ErrConflict
	}
	next := v.CurrentVersion + 1
	ver := domain.PaymentThemeVersion{ID: id + "-v" + fmt.Sprint(next), ThemeID: id, Version: next, Status: domain.ThemePublished, Config: v.DraftConfig, CreatedAt: now}
	if _, e = tx.ExecContext(ctx, `INSERT INTO payment_theme_versions(id,theme_id,version,schema_version,config,status,created_at)VALUES($1,$2,$3,1,$4,'PUBLISHED',$5)`, ver.ID, id, next, ver.Config, now); e != nil {
		return v, ver, e
	}
	v, e = scanTheme(tx.QueryRowContext(ctx, `UPDATE payment_themes SET status='PUBLISHED',current_version=$1,updated_at=$2 WHERE id=$3 RETURNING `+themeCols, next, now, id))
	if e != nil {
		return v, ver, e
	}
	return v, ver, tx.Commit()
}
func (r *Repository) ArchivePaymentTheme(ctx context.Context, tenant, id string, now time.Time) (domain.PaymentTheme, error) {
	query := `UPDATE payment_themes SET status='ARCHIVED',updated_at=$1 WHERE id=$2 AND tenant_id IS NULL AND is_default=false RETURNING ` + themeCols
	args := []any{now, id}
	if tenant != "" {
		query = `UPDATE payment_themes SET status='ARCHIVED',updated_at=$1 WHERE id=$2 AND tenant_id=$3 AND is_default=false RETURNING ` + themeCols
		args = []any{now, id, tenant}
	}
	v, e := scanTheme(r.DB.QueryRowContext(ctx, query, args...))
	return v, notFound(e)
}
func (r *Repository) SetDefaultPaymentTheme(ctx context.Context, tenant, id string, now time.Time) (domain.PaymentTheme, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return domain.PaymentTheme{}, e
	}
	defer tx.Rollback()
	if tenant == "" {
		if _, e = tx.ExecContext(ctx, `UPDATE payment_themes SET is_default=false WHERE tenant_id IS NULL`); e != nil {
			return domain.PaymentTheme{}, e
		}
	} else if _, e = tx.ExecContext(ctx, `UPDATE payment_themes SET is_default=false WHERE tenant_id=$1`, tenant); e != nil {
		return domain.PaymentTheme{}, e
	}
	query := `UPDATE payment_themes SET is_default=true,updated_at=$1 WHERE id=$2 AND tenant_id IS NULL AND status='PUBLISHED' RETURNING ` + themeCols
	args := []any{now, id}
	if tenant != "" {
		query = `UPDATE payment_themes SET is_default=true,updated_at=$1 WHERE id=$2 AND tenant_id=$3 AND status='PUBLISHED' RETURNING ` + themeCols
		args = []any{now, id, tenant}
	}
	v, e := scanTheme(tx.QueryRowContext(ctx, query, args...))
	if e != nil {
		return v, notFound(e)
	}
	return v, tx.Commit()
}
func (r *Repository) DuplicatePaymentTheme(ctx context.Context, tenant, source string, v domain.PaymentTheme) error {
	_, e := r.DB.ExecContext(ctx, `INSERT INTO payment_themes(id,tenant_id,name,status,is_default,current_version,draft_config,created_at,updated_at) SELECT $1,NULLIF($2,''),$3,'DRAFT',false,0,COALESCE(t.draft_config,tv.config),$4,$4 FROM payment_themes t LEFT JOIN payment_theme_versions tv ON tv.theme_id=t.id AND tv.version=t.current_version WHERE t.id=$5 AND (t.tenant_id=$2 OR t.tenant_id IS NULL)`, v.ID, tenant, v.Name, v.CreatedAt, source)
	return e
}
func (r *Repository) DeletePaymentTheme(ctx context.Context, tenant, id string) error {
	query := `DELETE FROM payment_themes WHERE id=$1 AND tenant_id IS NULL AND status='DRAFT' AND is_default=false`
	args := []any{id}
	if tenant != "" {
		query = `DELETE FROM payment_themes WHERE id=$1 AND tenant_id=$2 AND status='DRAFT' AND is_default=false`
		args = []any{id, tenant}
	}
	res, e := r.DB.ExecContext(ctx, query, args...)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrConflict
	}
	return nil
}

var _ = sql.ErrNoRows
