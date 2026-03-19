package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const tenantColumns = `id, name, created_at, updated_at`

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func scanTenant(row pgx.Row) (Tenant, error) {
	var tenant Tenant
	if err := row.Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	); err != nil {
		return Tenant{}, err
	}
	return tenant, nil
}

func (s *Store) CreateTenant(ctx context.Context, input TenantInput) (Tenant, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO tenants (name)
         VALUES ($1)
         RETURNING %s`, tenantColumns),
		input.Name,
	)
	tenant, err := scanTenant(row)
	if err != nil {
		return Tenant{}, err
	}
	return tenant, nil
}

func (s *Store) GetTenant(ctx context.Context, id uuid.UUID) (Tenant, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM tenants WHERE id = $1`, tenantColumns),
		id,
	)
	tenant, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tenant{}, NotFound("tenant")
		}
		return Tenant{}, err
	}
	return tenant, nil
}

func (s *Store) UpdateTenant(ctx context.Context, id uuid.UUID, update TenantUpdate) (Tenant, error) {
	builder := updateBuilder{}
	if update.Name != nil {
		builder.add("name", *update.Name)
	}

	if builder.empty() {
		return Tenant{}, fmt.Errorf("tenant update requires at least one field")
	}
	query, args := builder.build("tenants", tenantColumns, id)
	row := s.pool.QueryRow(ctx, query, args...)
	tenant, err := scanTenant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tenant{}, NotFound("tenant")
		}
		return Tenant{}, err
	}
	return tenant, nil
}

func (s *Store) DeleteTenant(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("tenant")
	}
	return nil
}

func (s *Store) ListTenants(ctx context.Context, _ TenantFilter, pageSize int32, cursor *PageCursor) (TenantListResult, error) {
	tenants, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM tenants", tenantColumns),
		nil,
		nil,
		cursor,
		pageSize,
		scanTenant,
		func(tenant Tenant) uuid.UUID { return tenant.ID },
	)
	if err != nil {
		return TenantListResult{}, err
	}
	return TenantListResult{Tenants: tenants, NextCursor: nextCursor}, nil
}

func (s *Store) GetTenantsByIDs(ctx context.Context, ids []uuid.UUID) ([]Tenant, error) {
	if len(ids) == 0 {
		return []Tenant{}, nil
	}
	idArray := pgtype.FlatArray[uuid.UUID](ids)
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf("SELECT %s FROM tenants WHERE id = ANY($1) ORDER BY id ASC", tenantColumns),
		idArray,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tenants := make([]Tenant, 0, len(ids))
	for rows.Next() {
		tenant, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, tenant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tenants, nil
}
