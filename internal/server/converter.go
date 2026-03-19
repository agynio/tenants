package server

import (
	tenantsv1 "github.com/agynio/tenants/.gen/go/agynio/api/tenants/v1"
	"github.com/agynio/tenants/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoTenant(tenant store.Tenant) *tenantsv1.Tenant {
	return &tenantsv1.Tenant{
		Id:        tenant.ID.String(),
		Name:      tenant.Name,
		CreatedAt: timestamppb.New(tenant.CreatedAt),
		UpdatedAt: timestamppb.New(tenant.UpdatedAt),
	}
}
