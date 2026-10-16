package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "open-prompts/backend/api/proto/v1"
)

// TestDashboardOverviewRequiresAuth verifies that GetDashboardOverview
// returns Unauthenticated when no user identity is in the context, before
// any repository call (the repository is nil here, so a repo call would
// panic — proving the auth check runs first).
func TestDashboardOverviewRequiresAuth(t *testing.T) {
	svc := NewDashboardService(nil, nil)
	_, err := svc.GetDashboardOverview(context.Background(), &pb.GetDashboardOverviewRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}
