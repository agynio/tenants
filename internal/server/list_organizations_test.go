package server

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	authorizationv1 "github.com/agynio/organizations/.gen/go/agynio/api/authorization/v1"
	organizationsv1 "github.com/agynio/organizations/.gen/go/agynio/api/organizations/v1"
	"github.com/agynio/organizations/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const authBufSize = 1024 * 1024

type authCheckServer struct {
	authorizationv1.UnimplementedAuthorizationServiceServer
	allowed     bool
	requestLock sync.Mutex
	lastRequest *authorizationv1.CheckRequest
}

func (s *authCheckServer) Check(ctx context.Context, req *authorizationv1.CheckRequest) (*authorizationv1.CheckResponse, error) {
	s.requestLock.Lock()
	s.lastRequest = req
	s.requestLock.Unlock()
	return &authorizationv1.CheckResponse{Allowed: s.allowed}, nil
}

func setupAuthClient(t *testing.T, allowed bool) (authorizationv1.AuthorizationServiceClient, *authCheckServer, func()) {
	t.Helper()

	lis := bufconn.Listen(authBufSize)
	grpcServer := grpc.NewServer()
	authServer := &authCheckServer{allowed: allowed}
	authorizationv1.RegisterAuthorizationServiceServer(grpcServer, authServer)
	go func() {
		_ = grpcServer.Serve(lis)
	}()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		t.Fatalf("dial bufnet: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		grpcServer.Stop()
	}
	return authorizationv1.NewAuthorizationServiceClient(conn), authServer, cleanup
}

func TestListOrganizationsClusterAdmin(t *testing.T) {
	authClient, authServer, cleanup := setupAuthClient(t, true)
	defer cleanup()

	identityID := uuid.New()
	organizationID := uuid.New()
	createdAt := time.Now().UTC()
	updatedAt := createdAt.Add(2 * time.Minute)
	called := false

	server := &Server{
		authorizationClient: authClient,
		listOrganizations: func(ctx context.Context, pageSize int32, cursor *store.PageCursor) (store.OrganizationListResult, error) {
			called = true
			return store.OrganizationListResult{
				Organizations: []store.Organization{
					{
						ID:        organizationID,
						Name:      "Acme Corp",
						CreatedAt: createdAt,
						UpdatedAt: updatedAt,
					},
				},
			}, nil
		},
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-identity-id", identityID.String()))
	response, err := server.ListOrganizations(ctx, &organizationsv1.ListOrganizationsRequest{PageSize: 5})
	if err != nil {
		t.Fatalf("ListOrganizations returned error: %v", err)
	}
	if !called {
		t.Fatal("expected listOrganizations to be called")
	}
	if len(response.Organizations) != 1 {
		t.Fatalf("expected 1 organization, got %d", len(response.Organizations))
	}
	if response.Organizations[0].GetId() != organizationID.String() {
		t.Fatalf("expected organization id %s, got %s", organizationID.String(), response.Organizations[0].GetId())
	}

	authServer.requestLock.Lock()
	request := authServer.lastRequest
	authServer.requestLock.Unlock()
	if request == nil || request.GetTupleKey() == nil {
		t.Fatal("expected authorization check request")
	}
	if request.GetTupleKey().GetObject() != clusterObject {
		t.Fatalf("expected cluster object %s, got %s", clusterObject, request.GetTupleKey().GetObject())
	}
	if request.GetTupleKey().GetRelation() != "admin" {
		t.Fatalf("expected admin relation, got %s", request.GetTupleKey().GetRelation())
	}
	if request.GetTupleKey().GetUser() != identityObjectPrefix+identityID.String() {
		t.Fatalf("expected user %s, got %s", identityObjectPrefix+identityID.String(), request.GetTupleKey().GetUser())
	}
}

func TestListOrganizationsNonAdminDenied(t *testing.T) {
	authClient, _, cleanup := setupAuthClient(t, false)
	defer cleanup()

	server := &Server{
		authorizationClient: authClient,
		listOrganizations: func(ctx context.Context, pageSize int32, cursor *store.PageCursor) (store.OrganizationListResult, error) {
			t.Fatal("listOrganizations should not be called for non-admin")
			return store.OrganizationListResult{}, nil
		},
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-identity-id", uuid.New().String()))
	_, err := server.ListOrganizations(ctx, &organizationsv1.ListOrganizationsRequest{PageSize: 5})
	if err == nil {
		t.Fatal("expected error for non-admin ListOrganizations")
	}
	statusErr, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %v", err)
	}
	if statusErr.Code() != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %s", statusErr.Code())
	}
}

func TestListOrganizationsMissingIdentityUnauthenticated(t *testing.T) {
	tests := map[string]context.Context{
		"no metadata":    context.Background(),
		"blank identity": metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-identity-id", "")),
	}

	for name, ctx := range tests {
		t.Run(name, func(t *testing.T) {
			server := &Server{
				listOrganizations: func(ctx context.Context, pageSize int32, cursor *store.PageCursor) (store.OrganizationListResult, error) {
					t.Fatal("listOrganizations should not be called without identity")
					return store.OrganizationListResult{}, nil
				},
			}

			_, err := server.ListOrganizations(ctx, &organizationsv1.ListOrganizationsRequest{PageSize: 5})
			if err == nil {
				t.Fatal("expected error for missing identity")
			}
			statusErr, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected status error, got %v", err)
			}
			if statusErr.Code() != codes.Unauthenticated {
				t.Fatalf("expected Unauthenticated, got %s", statusErr.Code())
			}
		})
	}
}
