package server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	authorizationv1 "github.com/agynio/tenants/.gen/go/agynio/api/authorization/v1"
	tenantsv1 "github.com/agynio/tenants/.gen/go/agynio/api/tenants/v1"
	"github.com/agynio/tenants/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const tenantObjectPrefix = "tenant:"

type Server struct {
	tenantsv1.UnimplementedTenantsServiceServer
	store               *store.Store
	authorizationClient authorizationv1.AuthorizationServiceClient
}

func New(store *store.Store, authorizationClient authorizationv1.AuthorizationServiceClient) *Server {
	return &Server{store: store, authorizationClient: authorizationClient}
}

func (s *Server) CreateTenant(ctx context.Context, req *tenantsv1.CreateTenantRequest) (*tenantsv1.CreateTenantResponse, error) {
	tenant, err := s.store.CreateTenant(ctx, store.TenantInput{Name: req.GetName()})
	if err != nil {
		return nil, toStatusError(err)
	}
	return &tenantsv1.CreateTenantResponse{Tenant: toProtoTenant(tenant)}, nil
}

func (s *Server) GetTenant(ctx context.Context, req *tenantsv1.GetTenantRequest) (*tenantsv1.GetTenantResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	tenant, err := s.store.GetTenant(ctx, id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &tenantsv1.GetTenantResponse{Tenant: toProtoTenant(tenant)}, nil
}

func (s *Server) UpdateTenant(ctx context.Context, req *tenantsv1.UpdateTenantRequest) (*tenantsv1.UpdateTenantResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if req.Name == nil {
		return nil, status.Error(codes.InvalidArgument, "at least one field must be provided")
	}

	update := store.TenantUpdate{}
	if req.Name != nil {
		value := req.GetName()
		update.Name = &value
	}

	tenant, err := s.store.UpdateTenant(ctx, id, update)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &tenantsv1.UpdateTenantResponse{Tenant: toProtoTenant(tenant)}, nil
}

func (s *Server) DeleteTenant(ctx context.Context, req *tenantsv1.DeleteTenantRequest) (*tenantsv1.DeleteTenantResponse, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id: %v", err)
	}
	if err := s.store.DeleteTenant(ctx, id); err != nil {
		return nil, toStatusError(err)
	}
	return &tenantsv1.DeleteTenantResponse{}, nil
}

func (s *Server) ListTenants(ctx context.Context, req *tenantsv1.ListTenantsRequest) (*tenantsv1.ListTenantsResponse, error) {
	cursor, err := decodePageCursor(req.GetPageToken())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
	}
	result, err := s.store.ListTenants(ctx, store.TenantFilter{}, req.GetPageSize(), cursor)
	if err != nil {
		return nil, toStatusError(err)
	}
	tenants, nextToken := mapListResult(result.Tenants, result.NextCursor, toProtoTenant)
	return &tenantsv1.ListTenantsResponse{Tenants: tenants, NextPageToken: nextToken}, nil
}

func (s *Server) ListAccessibleTenants(ctx context.Context, req *tenantsv1.ListAccessibleTenantsRequest) (*tenantsv1.ListAccessibleTenantsResponse, error) {
	identityID, err := parseUUID(req.GetIdentityId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity_id: %v", err)
	}

	authResponse, err := s.authorizationClient.ListObjects(ctx, &authorizationv1.ListObjectsRequest{
		Type:     "tenant",
		Relation: "member",
		User:     fmt.Sprintf("identity:%s", identityID.String()),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authorization list objects: %v", err)
	}
	if len(authResponse.Objects) == 0 {
		return &tenantsv1.ListAccessibleTenantsResponse{Tenants: []*tenantsv1.Tenant{}}, nil
	}

	tenantIDs := make([]uuid.UUID, 0, len(authResponse.Objects))
	for _, object := range authResponse.Objects {
		tenantID, err := parseTenantObject(object)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "authorization object %q: %v", object, err)
		}
		tenantIDs = append(tenantIDs, tenantID)
	}

	tenants, err := s.store.GetTenantsByIDs(ctx, tenantIDs)
	if err != nil {
		return nil, toStatusError(err)
	}
	protoTenants := make([]*tenantsv1.Tenant, len(tenants))
	for i, tenant := range tenants {
		protoTenants[i] = toProtoTenant(tenant)
	}
	return &tenantsv1.ListAccessibleTenantsResponse{Tenants: protoTenants}, nil
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

func parseTenantObject(value string) (uuid.UUID, error) {
	if !strings.HasPrefix(value, tenantObjectPrefix) {
		return uuid.UUID{}, fmt.Errorf("expected prefix %q", tenantObjectPrefix)
	}
	return parseUUID(strings.TrimPrefix(value, tenantObjectPrefix))
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
