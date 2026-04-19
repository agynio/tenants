package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	authorizationv1 "github.com/agynio/organizations/.gen/go/agynio/api/authorization/v1"
	identityv1 "github.com/agynio/organizations/.gen/go/agynio/api/identity/v1"
	organizationsv1 "github.com/agynio/organizations/.gen/go/agynio/api/organizations/v1"
	"github.com/agynio/organizations/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	organizationObjectPrefix = "organization:"
	identityObjectPrefix     = "identity:"
	clusterObject            = "cluster:global"
)

type Server struct {
	organizationsv1.UnimplementedOrganizationsServiceServer
	store               *store.Store
	authorizationClient authorizationv1.AuthorizationServiceClient
	identityClient      identityv1.IdentityServiceClient
}

func New(
	store *store.Store,
	authorizationClient authorizationv1.AuthorizationServiceClient,
	identityClient identityv1.IdentityServiceClient,
) *Server {
	return &Server{store: store, authorizationClient: authorizationClient, identityClient: identityClient}
}

func identityIDFromContext(ctx context.Context) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.Nil, fmt.Errorf("no metadata in context")
	}
	values := md.Get("x-identity-id")
	if len(values) == 0 || values[0] == "" {
		return uuid.Nil, fmt.Errorf("x-identity-id not found in metadata")
	}
	return uuid.Parse(values[0])
}

func (s *Server) checkPermission(ctx context.Context, identityID uuid.UUID, relation string, organizationID uuid.UUID) (bool, error) {
	response, err := s.authorizationClient.Check(ctx, &authorizationv1.CheckRequest{
		TupleKey: &authorizationv1.TupleKey{
			User:     fmt.Sprintf("%s%s", identityObjectPrefix, identityID.String()),
			Relation: relation,
			Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
		},
	})
	if err != nil {
		return false, err
	}
	return response.GetAllowed(), nil
}

func (s *Server) writeTuple(ctx context.Context, identityID uuid.UUID, relation string, organizationID uuid.UUID) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Writes: []*authorizationv1.TupleKey{
			{
				User:     fmt.Sprintf("%s%s", identityObjectPrefix, identityID.String()),
				Relation: relation,
				Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
			},
		},
	})
	return err
}

func (s *Server) deleteTuple(ctx context.Context, identityID uuid.UUID, relation string, organizationID uuid.UUID) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Deletes: []*authorizationv1.TupleKey{
			{
				User:     fmt.Sprintf("%s%s", identityObjectPrefix, identityID.String()),
				Relation: relation,
				Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
			},
		},
	})
	return err
}

func (s *Server) writeClusterTuple(ctx context.Context, relation string, organizationID uuid.UUID) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Writes: []*authorizationv1.TupleKey{
			{
				User:     clusterObject,
				Relation: relation,
				Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
			},
		},
	})
	return err
}

func (s *Server) deleteClusterTuple(ctx context.Context, relation string, organizationID uuid.UUID) error {
	_, err := s.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Deletes: []*authorizationv1.TupleKey{
			{
				User:     clusterObject,
				Relation: relation,
				Object:   fmt.Sprintf("%s%s", organizationObjectPrefix, organizationID.String()),
			},
		},
	})
	return err
}

func (s *Server) CreateOrganization(ctx context.Context, req *organizationsv1.CreateOrganizationRequest) (*organizationsv1.CreateOrganizationResponse, error) {
	identityID, err := identityIDFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "identity not available: %v", err)
	}

	organization, err := s.store.CreateOrganization(ctx, store.OrganizationInput{Name: req.GetName()})
	if err != nil {
		return nil, toStatusError(err)
	}

	if err := s.writeClusterTuple(ctx, "cluster", organization.ID); err != nil {
		_ = s.store.DeleteOrganization(ctx, organization.ID)
		return nil, status.Errorf(codes.Internal, "failed to write cluster tuple: %v", err)
	}

	if err := s.writeTuple(ctx, identityID, "owner", organization.ID); err != nil {
		_ = s.deleteClusterTuple(ctx, "cluster", organization.ID)
		_ = s.store.DeleteOrganization(ctx, organization.ID)
		return nil, status.Errorf(codes.Internal, "failed to write ownership tuple: %v", err)
	}

	_, err = s.store.CreateMembership(ctx, store.MembershipInput{
		OrganizationID: organization.ID,
		IdentityID:     identityID,
		Role:           store.MembershipRoleOwner,
		Status:         store.MembershipStatusActive,
	})
	if err != nil {
		_ = s.deleteTuple(ctx, identityID, "owner", organization.ID)
		_ = s.deleteClusterTuple(ctx, "cluster", organization.ID)
		_ = s.store.DeleteOrganization(ctx, organization.ID)
		return nil, toStatusError(err)
	}
	return &organizationsv1.CreateOrganizationResponse{Organization: toProtoOrganization(organization)}, nil
}

func (s *Server) GetOrganization(ctx context.Context, req *organizationsv1.GetOrganizationRequest) (*organizationsv1.GetOrganizationResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	organization, err := s.store.GetOrganization(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &organizationsv1.GetOrganizationResponse{Organization: toProtoOrganization(organization)}, nil
}

func (s *Server) UpdateOrganization(ctx context.Context, req *organizationsv1.UpdateOrganizationRequest) (*organizationsv1.UpdateOrganizationResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.OrganizationUpdate{}
	if req.Name != nil {
		value := req.GetName()
		update.Name = &value
	}

	organization, err := s.store.UpdateOrganization(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &organizationsv1.UpdateOrganizationResponse{Organization: toProtoOrganization(organization)}, nil
}

func (s *Server) DeleteOrganization(ctx context.Context, req *organizationsv1.DeleteOrganizationRequest) (*organizationsv1.DeleteOrganizationResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteOrganization(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &organizationsv1.DeleteOrganizationResponse{}, nil
}

func (s *Server) ListOrganizations(ctx context.Context, req *organizationsv1.ListOrganizationsRequest) (*organizationsv1.ListOrganizationsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListOrganizations(ctx, store.OrganizationFilter{}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	organizations, nextToken := mapListResult(result.Organizations, result.NextCursor, toProtoOrganization)
	return &organizationsv1.ListOrganizationsResponse{Organizations: organizations, NextPageToken: nextToken}, nil
}

func (s *Server) ListAccessibleOrganizations(ctx context.Context, req *organizationsv1.ListAccessibleOrganizationsRequest) (*organizationsv1.ListAccessibleOrganizationsResponse, error) {
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}

	memberResponse, err := s.authorizationClient.ListObjects(ctx, &authorizationv1.ListObjectsRequest{
		Type:     "organization",
		Relation: "member",
		User:     fmt.Sprintf("%s%s", identityObjectPrefix, identityID.String()),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization list objects: %v", err)
	}
	adminResponse, err := s.authorizationClient.ListObjects(ctx, &authorizationv1.ListObjectsRequest{
		Type:     "organization",
		Relation: "can_add_member",
		User:     fmt.Sprintf("%s%s", identityObjectPrefix, identityID.String()),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization list objects: %v", err)
	}

	objects := make(map[string]struct{}, len(memberResponse.Objects)+len(adminResponse.Objects))
	for _, object := range memberResponse.Objects {
		objects[object] = struct{}{}
	}
	for _, object := range adminResponse.Objects {
		objects[object] = struct{}{}
	}
	if len(objects) == 0 {
		return &organizationsv1.ListAccessibleOrganizationsResponse{Organizations: []*organizationsv1.Organization{}}, nil
	}

	organizationIDs := make([]uuid.UUID, 0, len(objects))
	for object := range objects {
		organizationID, err := parseOrganizationObject(object)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "authorization object %q: %v", object, err)
		}
		organizationIDs = append(organizationIDs, organizationID)
	}

	organizations, err := s.store.GetOrganizationsByIDs(ctx, organizationIDs)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoOrganizations := make([]*organizationsv1.Organization, len(organizations))
	for i, organization := range organizations {
		protoOrganizations[i] = toProtoOrganization(organization)
	}
	return &organizationsv1.ListAccessibleOrganizationsResponse{Organizations: protoOrganizations}, nil
}

func decodePageCursor(token string) (*store.PageCursor, error) {
	if token == "" {
		return nil, nil
	}
	id, err := store.DecodePageToken(token)
	if err != nil {
		return nil, err
	}
	return &store.PageCursor{AfterID: id}, nil
}

func mapListResult[T any, P any](items []T, nextCursor *store.PageCursor, convert func(T) P) ([]P, string) {
	results := make([]P, len(items))
	for i, item := range items {
		results[i] = convert(item)
	}
	if nextCursor == nil {
		return results, ""
	}
	return results, store.EncodePageToken(nextCursor.AfterID)
}

func parseUUID(value string) (uuid.UUID, error) {
	if value == "" {
		return uuid.UUID{}, fmt.Errorf("value is empty")
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, err
	}
	return id, nil
}

func parseOrganizationObject(value string) (uuid.UUID, error) {
	if !strings.HasPrefix(value, organizationObjectPrefix) {
		return uuid.UUID{}, fmt.Errorf("expected prefix %q", organizationObjectPrefix)
	}
	return parseUUID(strings.TrimPrefix(value, organizationObjectPrefix))
}

func toStatusError(err error) error {
	var notFound *store.NotFoundError
	if errors.As(err, &notFound) {
		return status.Error(codes.NotFound, notFound.Error())
	}
	var exists *store.AlreadyExistsError
	if errors.As(err, &exists) {
		return status.Error(codes.AlreadyExists, exists.Error())
	}
	var foreignKey *store.ForeignKeyViolationError
	if errors.As(err, &foreignKey) {
		return status.Error(codes.FailedPrecondition, foreignKey.Error())
	}
	return status.Errorf(codes.Internal, "internal error: %v", err)
}
