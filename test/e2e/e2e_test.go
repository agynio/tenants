//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	tenantsv1 "github.com/agynio/tenants/.gen/go/agynio/api/tenants/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const listPageSize int32 = 50

func TestTenantsServiceE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := grpc.DialContext(ctx, tenantsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := tenantsv1.NewTenantsServiceClient(conn)

	testID := uuid.NewString()
	tenantResp1, err := client.CreateTenant(ctx, &tenantsv1.CreateTenantRequest{Name: "Tenant Alpha " + testID})
	require.NoError(t, err)
	tenantID1 := tenantResp1.Tenant.Id

	tenantResp2, err := client.CreateTenant(ctx, &tenantsv1.CreateTenantRequest{Name: "Tenant Beta " + testID})
	require.NoError(t, err)
	tenantID2 := tenantResp2.Tenant.Id

	getResp, err := client.GetTenant(ctx, &tenantsv1.GetTenantRequest{Id: tenantID1})
	require.NoError(t, err)
	require.Equal(t, tenantID1, getResp.Tenant.Id)

	updatedTenantResp, err := client.UpdateTenant(ctx, &tenantsv1.UpdateTenantRequest{
		Id:   tenantID1,
		Name: proto.String("Tenant Alpha Updated " + testID),
	})
	require.NoError(t, err)
	require.Equal(t, "Tenant Alpha Updated "+testID, updatedTenantResp.Tenant.Name)

	listResp, err := client.ListTenants(ctx, &tenantsv1.ListTenantsRequest{PageSize: 1})
	require.NoError(t, err)
	require.NotEmpty(t, listResp.Tenants)
	require.NotEmpty(t, listResp.NextPageToken)

	tenants := listTenants(ctx, t, client)
	require.True(t, hasID(tenants, tenantID1))
	require.True(t, hasID(tenants, tenantID2))

	_, err = client.UpdateTenant(ctx, &tenantsv1.UpdateTenantRequest{Id: tenantID1})
	requireStatusCode(t, err, codes.InvalidArgument)

	_, err = client.GetTenant(ctx, &tenantsv1.GetTenantRequest{Id: uuid.NewString()})
	requireStatusCode(t, err, codes.NotFound)

	_, err = client.DeleteTenant(ctx, &tenantsv1.DeleteTenantRequest{Id: tenantID2})
	require.NoError(t, err)
	_, err = client.DeleteTenant(ctx, &tenantsv1.DeleteTenantRequest{Id: tenantID1})
	require.NoError(t, err)
}

func listPaged[T any](t *testing.T, resource string, fetch func(pageToken string) ([]T, string, error)) []T {
	t.Helper()
	var items []T
	pageToken := ""
	for i := 0; i < 20; i++ {
		pageItems, nextPageToken, err := fetch(pageToken)
		require.NoError(t, err)
		items = append(items, pageItems...)
		if nextPageToken == "" {
			return items
		}
		pageToken = nextPageToken
	}
	t.Fatalf("%s pagination exceeded", resource)
	return nil
}

func listTenants(ctx context.Context, t *testing.T, client tenantsv1.TenantsServiceClient) []*tenantsv1.Tenant {
	return listPaged(t, "tenant", func(pageToken string) ([]*tenantsv1.Tenant, string, error) {
		resp, err := client.ListTenants(ctx, &tenantsv1.ListTenantsRequest{PageSize: listPageSize, PageToken: pageToken})
		if err != nil {
			return nil, "", err
		}
		return resp.Tenants, resp.NextPageToken, nil
	})
}

func hasID(items []*tenantsv1.Tenant, id string) bool {
	for _, item := range items {
		if item.GetId() == id {
			return true
		}
	}
	return false
}

func requireStatusCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	statusErr, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, code, statusErr.Code())
}
