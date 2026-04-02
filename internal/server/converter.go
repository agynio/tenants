package server

import (
	"fmt"

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

func toStoreMembershipRole(role organizationsv1.MembershipRole) (store.MembershipRole, error) {
	switch role {
	case organizationsv1.MembershipRole_MEMBERSHIP_ROLE_OWNER:
		return store.MembershipRoleOwner, nil
	case organizationsv1.MembershipRole_MEMBERSHIP_ROLE_MEMBER:
		return store.MembershipRoleMember, nil
	default:
		return "", fmt.Errorf("invalid membership role: %v", role)
	}
}

func toStoreMembershipStatus(status organizationsv1.MembershipStatus) *store.MembershipStatus {
	switch status {
	case organizationsv1.MembershipStatus_MEMBERSHIP_STATUS_PENDING:
		value := store.MembershipStatusPending
		return &value
	case organizationsv1.MembershipStatus_MEMBERSHIP_STATUS_ACTIVE:
		value := store.MembershipStatusActive
		return &value
	default:
		return nil
	}
}

func toProtoMembershipRole(role store.MembershipRole) organizationsv1.MembershipRole {
	switch role {
	case store.MembershipRoleOwner:
		return organizationsv1.MembershipRole_MEMBERSHIP_ROLE_OWNER
	case store.MembershipRoleMember:
		return organizationsv1.MembershipRole_MEMBERSHIP_ROLE_MEMBER
	default:
		panic(fmt.Sprintf("unexpected membership role: %q", role))
	}
}

func toProtoMembershipStatus(status store.MembershipStatus) organizationsv1.MembershipStatus {
	switch status {
	case store.MembershipStatusPending:
		return organizationsv1.MembershipStatus_MEMBERSHIP_STATUS_PENDING
	case store.MembershipStatusActive:
		return organizationsv1.MembershipStatus_MEMBERSHIP_STATUS_ACTIVE
	default:
		panic(fmt.Sprintf("unexpected membership status: %q", status))
	}
}

func toProtoMembership(membership store.Membership) *organizationsv1.Membership {
	var expiresAt *timestamppb.Timestamp
	if membership.ExpiresAt != nil {
		expiresAt = timestamppb.New(*membership.ExpiresAt)
	}
	return &organizationsv1.Membership{
		Id:             membership.ID.String(),
		OrganizationId: membership.OrganizationID.String(),
		IdentityId:     membership.IdentityID.String(),
		Role:           toProtoMembershipRole(membership.Role),
		Status:         toProtoMembershipStatus(membership.Status),
		ExpiresAt:      expiresAt,
		CreatedAt:      timestamppb.New(membership.CreatedAt),
		UpdatedAt:      timestamppb.New(membership.UpdatedAt),
	}
}
