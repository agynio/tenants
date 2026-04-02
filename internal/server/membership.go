package server

import (
	"context"
	"time"

	authorizationv1 "github.com/agynio/organizations/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/organizations/.gen/go/agynio/api/identity/v1"
	organizationsv1 "github.com/agynio/organizations/.gen/go/agynio/api/organizations/v1"
	"github.com/agynio/organizations/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) CreateMembership(ctx context.Context, req *organizationsv1.CreateMembershipRequest) (*organizationsv1.CreateMembershipResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}
	role, err := toStoreMembershipRole(req.GetRole())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "role: %v", err)
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		if err := req.ExpiresAt.CheckValid(); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "expires_at: %v", err)
		}
		value := req.ExpiresAt.AsTime()
		expiresAt = &value
	}

	if err := s.ensureIdentityExists(ctx, identityID); err != nil {
		return nil, err
	}

	allowed, err := s.checkPermission(ctx, callerID, "can_add_member", organizationID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	statusValue := store.MembershipStatusPending
	if allowed {
		statusValue = store.MembershipStatusActive
	} else {
		invited, err := s.checkPermission(ctx, callerID, "can_invite", organizationID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "authorization check: %v", err)
		}
		if !invited {
			return nil, status.Error(codes.PermissionDenied, "missing permission to add or invite members")
		}
	}

	membership, err := s.store.CreateMembership(ctx, store.MembershipInput{
		OrganizationID: organizationID,
		IdentityID:     identityID,
		Role:           role,
		Status:         statusValue,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return nil, toStatusError(err)
	}

	if membership.Status == store.MembershipStatusActive {
		if err := s.writeTuple(ctx, membership.IdentityID, string(membership.Role), membership.OrganizationID); err != nil {
			_ = s.store.DeleteMembership(ctx, membership.ID)
			return nil, status.Errorf(codes.Internal, "failed to write membership tuple: %v", err)
		}
	}

	return &organizationsv1.CreateMembershipResponse{Membership: toProtoMembership(membership)}, nil
}

func (s *Server) AcceptMembership(ctx context.Context, req *organizationsv1.AcceptMembershipRequest) (*organizationsv1.AcceptMembershipResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	membershipID, err := parseUUID(req.GetMembershipId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "membership_id: %v", err)
	}

	membership, err := s.store.GetMembership(ctx, membershipID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if membership.IdentityID != callerID {
		return nil, status.Error(codes.PermissionDenied, "membership belongs to a different identity")
	}
	if membership.Status != store.MembershipStatusPending {
		return nil, status.Error(codes.FailedPrecondition, "membership is not pending")
	}

	updated, err := s.store.UpdateMembershipStatus(ctx, membership.ID, store.MembershipStatusActive)
	if err != nil {
		return nil, toStatusError(err)
	}

	if err := s.writeTuple(ctx, updated.IdentityID, string(updated.Role), updated.OrganizationID); err != nil {
		_, _ = s.store.UpdateMembershipStatus(ctx, updated.ID, store.MembershipStatusPending)
		return nil, status.Errorf(codes.Internal, "failed to write membership tuple: %v", err)
	}

	return &organizationsv1.AcceptMembershipResponse{Membership: toProtoMembership(updated)}, nil
}

func (s *Server) DeclineMembership(ctx context.Context, req *organizationsv1.DeclineMembershipRequest) (*organizationsv1.DeclineMembershipResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	membershipID, err := parseUUID(req.GetMembershipId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "membership_id: %v", err)
	}

	membership, err := s.store.GetMembership(ctx, membershipID)
	if err != nil {
		return nil, toStatusError(err)
	}
	if membership.IdentityID != callerID {
		return nil, status.Error(codes.PermissionDenied, "membership belongs to a different identity")
	}
	if membership.Status != store.MembershipStatusPending {
		return nil, status.Error(codes.FailedPrecondition, "membership is not pending")
	}

	if err := s.store.DeleteMembership(ctx, membership.ID); err != nil {
		return nil, toStatusError(err)
	}

	return &organizationsv1.DeclineMembershipResponse{}, nil
}

func (s *Server) RemoveMembership(ctx context.Context, req *organizationsv1.RemoveMembershipRequest) (*organizationsv1.RemoveMembershipResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	membershipID, err := parseUUID(req.GetMembershipId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "membership_id: %v", err)
	}

	membership, err := s.store.GetMembership(ctx, membershipID)
	if err != nil {
		return nil, toStatusError(err)
	}

	allowed, err := s.checkPermission(ctx, callerID, "can_manage_members", membership.OrganizationID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "missing permission to manage members")
	}

	if membership.Status == store.MembershipStatusActive {
		if err := s.deleteTuple(ctx, membership.IdentityID, string(membership.Role), membership.OrganizationID); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to delete membership tuple: %v", err)
		}
	}

	if err := s.store.DeleteMembership(ctx, membership.ID); err != nil {
		return nil, toStatusError(err)
	}

	return &organizationsv1.RemoveMembershipResponse{}, nil
}

func (s *Server) UpdateMembershipRole(ctx context.Context, req *organizationsv1.UpdateMembershipRoleRequest) (*organizationsv1.UpdateMembershipRoleResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	membershipID, err := parseUUID(req.GetMembershipId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "membership_id: %v", err)
	}

	role, err := toStoreMembershipRole(req.GetRole())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "role: %v", err)
	}

	membership, err := s.store.GetMembership(ctx, membershipID)
	if err != nil {
		return nil, toStatusError(err)
	}

	allowed, err := s.checkPermission(ctx, callerID, "can_manage_members", membership.OrganizationID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "missing permission to manage members")
	}

	updated, err := s.store.UpdateMembershipRole(ctx, membership.ID, role)
	if err != nil {
		return nil, toStatusError(err)
	}

	if updated.Status == store.MembershipStatusActive {
		_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
			Writes: []*authorizationv1.TupleKey{
				{
					User:     identityObjectPrefix + updated.IdentityID.String(),
					Relation: string(updated.Role),
					Object:   organizationObjectPrefix + updated.OrganizationID.String(),
				},
			},
			Deletes: []*authorizationv1.TupleKey{
				{
					User:     identityObjectPrefix + membership.IdentityID.String(),
					Relation: string(membership.Role),
					Object:   organizationObjectPrefix + membership.OrganizationID.String(),
				},
			},
		})
		if err != nil {
			_, _ = s.store.UpdateMembershipRole(ctx, membership.ID, membership.Role)
			return nil, status.Errorf(codes.Internal, "failed to update membership tuple: %v", err)
		}
	}

	return &organizationsv1.UpdateMembershipRoleResponse{Membership: toProtoMembership(updated)}, nil
}

func (s *Server) ListMembers(ctx context.Context, req *organizationsv1.ListMembersRequest) (*organizationsv1.ListMembersResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	organizationID, err := parseUUID(req.GetOrganizationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "organization_id: %v", err)
	}

	allowed, err := s.checkPermission(ctx, callerID, "can_manage_members", organizationID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization check: %v", err)
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "missing permission to manage members")
	}

	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	result, err := s.store.ListMemberships(ctx, store.MembershipFilter{
		OrganizationID: &organizationID,
		Status:         toStoreMembershipStatus(req.GetStatus()),
	}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	memberships, nextToken := mapListResult(result.Memberships, result.NextCursor, toProtoMembership)
	return &organizationsv1.ListMembersResponse{Memberships: memberships, NextPageToken: nextToken}, nil
}

func (s *Server) ListMyMemberships(ctx context.Context, req *organizationsv1.ListMyMembershipsRequest) (*organizationsv1.ListMyMembershipsResponse, error) {
	callerID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}

	result, err := s.store.ListMemberships(ctx, store.MembershipFilter{
		IdentityID: &callerID,
		Status:     toStoreMembershipStatus(req.GetStatus()),
	}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}

	memberships, nextToken := mapListResult(result.Memberships, result.NextCursor, toProtoMembership)
	return &organizationsv1.ListMyMembershipsResponse{Memberships: memberships, NextPageToken: nextToken}, nil
}

func (s *Server) ensureIdentityExists(ctx context.Context, identityID uuid.UUID) error {
	_, err := s.identityClient.GetIdentityType(ctx, &identityv1.GetIdentityTypeRequest{IdentityId: identityID.String()})
	if err == nil {
		return nil
	}
	statusErr, ok := status.FromError(err)
	if ok && statusErr.Code() == codes.NotFound {
		return status.Error(codes.FailedPrecondition, "identity not found")
	}
	return status.Errorf(codes.Internal, "identity lookup failed: %v", err)
}
