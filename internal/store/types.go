package store

import (
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TenantInput struct {
	Name string
}

type TenantUpdate struct {
	Name *string
}

type TenantFilter struct{}

type PageCursor struct {
	AfterID uuid.UUID
}

type TenantListResult struct {
	Tenants    []Tenant
	NextCursor *PageCursor
}
