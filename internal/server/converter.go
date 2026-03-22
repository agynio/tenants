package server

import (
	organizationsv1 "github.com/agynio/organizations/.gen/go/agynio/api/organizations/v1"
	"github.com/agynio/organizations/internal/store"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoOrganization(organization store.Organization) *organizationsv1.Organization {
	return &organizationsv1.Organization{
		Id:        organization.ID.String(),
		Name:      organization.Name,
		CreatedAt: timestamppb.New(organization.CreatedAt),
		UpdatedAt: timestamppb.New(organization.UpdatedAt),
	}
}
