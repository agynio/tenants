//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	organizationsv1 "github.com/agynio/organizations/.gen/go/agynio/api/organizations/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const listPageSize int32 = 50

func TestOrganizationsServiceE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := grpc.DialContext(ctx, organizationsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := organizationsv1.NewOrganizationsServiceClient(conn)

	testID := uuid.NewString()
	organizationResp1, err := client.CreateOrganization(ctx, &organizationsv1.CreateOrganizationRequest{Name: "Organization Alpha " + testID})
	require.NoError(t, err)
	organizationID1 := organizationResp1.Organization.Id

	organizationResp2, err := client.CreateOrganization(ctx, &organizationsv1.CreateOrganizationRequest{Name: "Organization Beta " + testID})
	require.NoError(t, err)
	organizationID2 := organizationResp2.Organization.Id

	getResp, err := client.GetOrganization(ctx, &organizationsv1.GetOrganizationRequest{Id: organizationID1})
	require.NoError(t, err)
	require.Equal(t, organizationID1, getResp.Organization.Id)

	updatedOrganizationResp, err := client.UpdateOrganization(ctx, &organizationsv1.UpdateOrganizationRequest{
		Id:   organizationID1,
		Name: proto.String("Organization Alpha Updated " + testID),
	})
	require.NoError(t, err)
	require.Equal(t, "Organization Alpha Updated "+testID, updatedOrganizationResp.Organization.Name)

	listResp, err := client.ListOrganizations(ctx, &organizationsv1.ListOrganizationsRequest{PageSize: 1})
	require.NoError(t, err)
	require.NotEmpty(t, listResp.Organizations)
	require.NotEmpty(t, listResp.NextPageToken)

	organizations := listOrganizations(ctx, t, client)
	require.True(t, hasID(organizations, organizationID1))
	require.True(t, hasID(organizations, organizationID2))

	_, err = client.UpdateOrganization(ctx, &organizationsv1.UpdateOrganizationRequest{Id: organizationID1})
	requireStatusCode(t, err, codes.InvalidArgument)

	_, err = client.GetOrganization(ctx, &organizationsv1.GetOrganizationRequest{Id: uuid.NewString()})
	requireStatusCode(t, err, codes.NotFound)

	_, err = client.DeleteOrganization(ctx, &organizationsv1.DeleteOrganizationRequest{Id: organizationID2})
	require.NoError(t, err)
	_, err = client.DeleteOrganization(ctx, &organizationsv1.DeleteOrganizationRequest{Id: organizationID1})
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

func listOrganizations(ctx context.Context, t *testing.T, client organizationsv1.OrganizationsServiceClient) []*organizationsv1.Organization {
	return listPaged(t, "organization", func(pageToken string) ([]*organizationsv1.Organization, string, error) {
		resp, err := client.ListOrganizations(ctx, &organizationsv1.ListOrganizationsRequest{PageSize: listPageSize, PageToken: pageToken})
		if err != nil {
			return nil, "", err
		}
		return resp.Organizations, resp.NextPageToken, nil
	})
}

func hasID(items []*organizationsv1.Organization, id string) bool {
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
