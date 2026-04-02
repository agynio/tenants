package store

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OrganizationInput struct {
	Name string
}

type OrganizationUpdate struct {
	Name *string
}

type OrganizationFilter struct{}

type PageCursor struct {
	AfterID uuid.UUID
}

type OrganizationListResult struct {
	Organizations []Organization
	NextCursor    *PageCursor
}

type MembershipRole string

const (
	MembershipRoleOwner  MembershipRole = "owner"
	MembershipRoleMember MembershipRole = "member"
)

type MembershipStatus string

const (
	MembershipStatusPending MembershipStatus = "pending"
	MembershipStatusActive  MembershipStatus = "active"
)

type Membership struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	IdentityID     uuid.UUID
	Role           MembershipRole
	Status         MembershipStatus
	ExpiresAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type MembershipInput struct {
	OrganizationID uuid.UUID
	IdentityID     uuid.UUID
	Role           MembershipRole
	Status         MembershipStatus
	ExpiresAt      *time.Time
}

type MembershipFilter struct {
	OrganizationID *uuid.UUID
	IdentityID     *uuid.UUID
	Status         *MembershipStatus
}

type MembershipListResult struct {
	Memberships []Membership
	NextCursor  *PageCursor
}
