package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const membershipColumns = `id, organization_id, identity_id, role, status, expires_at, created_at, updated_at`

func scanMembership(row pgx.Row) (Membership, error) {
	var (
		membership Membership
		expiresAt  pgtype.Timestamptz
	)
	if err := row.Scan(
		&membership.ID,
		&membership.OrganizationID,
		&membership.IdentityID,
		&membership.Role,
		&membership.Status,
		&expiresAt,
		&membership.CreatedAt,
		&membership.UpdatedAt,
	); err != nil {
		return Membership{}, err
	}
	if expiresAt.Valid {
		membership.ExpiresAt = &expiresAt.Time
	}
	return membership, nil
}

func (s *Store) CreateMembership(ctx context.Context, input MembershipInput) (Membership, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO memberships (organization_id, identity_id, role, status, expires_at)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING %s`, membershipColumns),
		input.OrganizationID,
		input.IdentityID,
		input.Role,
		input.Status,
		input.ExpiresAt,
	)
	membership, err := scanMembership(row)
	if err != nil {
		if isUniqueViolation(err) {
			return Membership{}, AlreadyExists("membership")
		}
		if isForeignKeyViolation(err) {
			return Membership{}, ForeignKeyViolation("membership")
		}
		return Membership{}, err
	}
	return membership, nil
}

func (s *Store) GetMembership(ctx context.Context, id uuid.UUID) (Membership, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM memberships WHERE id = $1`, membershipColumns),
		id,
	)
	membership, err := scanMembership(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Membership{}, NotFound("membership")
		}
		return Membership{}, err
	}
	return membership, nil
}

func (s *Store) GetMembershipByOrganizationIdentity(ctx context.Context, organizationID uuid.UUID, identityID uuid.UUID) (Membership, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM memberships WHERE organization_id = $1 AND identity_id = $2`, membershipColumns),
		organizationID,
		identityID,
	)
	membership, err := scanMembership(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Membership{}, NotFound("membership")
		}
		return Membership{}, err
	}
	return membership, nil
}

func (s *Store) UpdateMembershipStatus(ctx context.Context, id uuid.UUID, status MembershipStatus) (Membership, error) {
	builder := updateBuilder{}
	builder.add("status", status)
	query, args := builder.build("memberships", membershipColumns, id)
	row := s.pool.QueryRow(ctx, query, args...)
	membership, err := scanMembership(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Membership{}, NotFound("membership")
		}
		return Membership{}, err
	}
	return membership, nil
}

func (s *Store) UpdateMembershipRole(ctx context.Context, id uuid.UUID, role MembershipRole) (Membership, error) {
	builder := updateBuilder{}
	builder.add("role", role)
	query, args := builder.build("memberships", membershipColumns, id)
	row := s.pool.QueryRow(ctx, query, args...)
	membership, err := scanMembership(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Membership{}, NotFound("membership")
		}
		return Membership{}, err
	}
	return membership, nil
}

func (s *Store) DeleteMembership(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM memberships WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return NotFound("membership")
	}
	return nil
}

func (s *Store) ListMemberships(ctx context.Context, filter MembershipFilter, pageSize int32, cursor *PageCursor) (MembershipListResult, error) {
	var (
		clauses []string
		args    []any
	)
	if filter.OrganizationID != nil {
		clauses, args = appendClause(clauses, args, "organization_id = $%d", *filter.OrganizationID)
	}
	if filter.IdentityID != nil {
		clauses, args = appendClause(clauses, args, "identity_id = $%d", *filter.IdentityID)
	}
	if filter.Status != nil {
		clauses, args = appendClause(clauses, args, "status = $%d", *filter.Status)
	}

	memberships, nextCursor, err := listEntities(ctx, s.pool,
		fmt.Sprintf("SELECT %s FROM memberships", membershipColumns),
		clauses,
		args,
		cursor,
		pageSize,
		scanMembership,
		func(membership Membership) uuid.UUID { return membership.ID },
	)
	if err != nil {
		return MembershipListResult{}, err
	}
	return MembershipListResult{Memberships: memberships, NextCursor: nextCursor}, nil
}
