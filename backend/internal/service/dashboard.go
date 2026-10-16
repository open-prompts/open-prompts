package service

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/repository"
)

// DashboardService serves the authenticated user's personal workspace.
type DashboardService struct {
	pb.UnimplementedDashboardServiceServer

	dashboardRepo *repository.DashboardRepository
	apiKeyRepo    *repository.APIKeyRepository
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(
	dashboardRepo *repository.DashboardRepository,
	apiKeyRepo *repository.APIKeyRepository,
) *DashboardService {
	return &DashboardService{
		dashboardRepo: dashboardRepo,
		apiKeyRepo:    apiKeyRepo,
	}
}

// GetDashboardOverview returns the caller's resource counts and recent
// activity. Identity is derived from the auth context only.
func (s *DashboardService) GetDashboardOverview(ctx context.Context, req *pb.GetDashboardOverviewRequest) (*pb.GetDashboardOverviewResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err // Unauthenticated
	}

	overview, err := s.dashboardRepo.GetOverview(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load dashboard: %v", err)
	}

	activity := make([]*pb.DashboardActivityItem, 0, len(overview.Activity))
	for _, a := range overview.Activity {
		item := &pb.DashboardActivityItem{
			EntityId:    a.EntityID,
			EntityTitle: a.EntityTitle,
			Timestamp:   timestamppb.New(a.Timestamp),
		}
		switch a.Type {
		case repository.DashboardActivityTemplate:
			item.Type = pb.DashboardActivityType_DASHBOARD_ACTIVITY_TYPE_TEMPLATE
		case repository.DashboardActivityPrompt:
			item.Type = pb.DashboardActivityType_DASHBOARD_ACTIVITY_TYPE_PROMPT
			item.TemplateId = a.TemplateID
		case repository.DashboardActivityLike:
			item.Type = pb.DashboardActivityType_DASHBOARD_ACTIVITY_TYPE_LIKE
		case repository.DashboardActivityFavorite:
			item.Type = pb.DashboardActivityType_DASHBOARD_ACTIVITY_TYPE_FAVORITE
		}
		activity = append(activity, item)
	}

	return &pb.GetDashboardOverviewResponse{
		TemplateCount:   overview.TemplateCount,
		PromptCount:     overview.PromptCount,
		CollectionCount: overview.CollectionCount,
		ApiKeyCount:     overview.APIKeyCount,
		RecentActivity:  activity,
	}, nil
}
