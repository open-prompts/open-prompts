package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"
)

const (
	defaultNotificationReadRetentionDays = 90
	defaultNotificationRetentionDays     = 180
	notificationPurgeInterval            = 10 * time.Minute
	notificationMaxPageSize              = 100
	notificationDefaultPageSize          = 20
)

// NotificationService implements the per-user engagement notification RPCs.
// The recipient is always derived from the authenticated context.
type NotificationService struct {
	pb.UnimplementedNotificationServiceServer

	repo              repository.NotificationRepository
	readRetentionDays int
	retentionDays     int
	lastPurgeAt       time.Time
	purgeMu           sync.Mutex
}

// NewNotificationService creates a new NotificationService. Non-positive
// retention values fall back to the documented defaults (90/180 days).
func NewNotificationService(repo repository.NotificationRepository, readRetentionDays, retentionDays int) *NotificationService {
	if readRetentionDays <= 0 {
		readRetentionDays = defaultNotificationReadRetentionDays
	}
	if retentionDays <= 0 {
		retentionDays = defaultNotificationRetentionDays
	}
	return &NotificationService{
		repo:              repo,
		readRetentionDays: readRetentionDays,
		retentionDays:     retentionDays,
	}
}

// maybePurge runs the lazy retention delete at most once per interval per
// instance. Failures are logged and never block the caller.
func (s *NotificationService) maybePurge(ctx context.Context) {
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()
	if time.Since(s.lastPurgeAt) < notificationPurgeInterval {
		return
	}
	s.lastPurgeAt = time.Now()
	if err := s.repo.PurgeExpired(ctx, s.readRetentionDays, s.retentionDays); err != nil {
		zap.S().Errorf("NotificationService: lazy purge failed: %v", err)
	}
}

// ListNotifications returns a page of the caller's notifications plus the
// unread count so the badge can be reconciled from a single call.
func (s *NotificationService) ListNotifications(ctx context.Context, req *pb.ListNotificationsRequest) (*pb.ListNotificationsResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	filter := req.Filter
	if filter == "" {
		filter = "all"
	}
	if filter != "all" && filter != "unread" {
		return nil, status.Error(codes.InvalidArgument, "filter must be 'all' or 'unread'")
	}

	pageSize := int(req.PageSize)
	if req.PageSize == 0 {
		pageSize = notificationDefaultPageSize
	}
	if pageSize < 1 || pageSize > notificationMaxPageSize {
		return nil, status.Error(codes.InvalidArgument, "page_size must be between 1 and 100")
	}

	offset := 0
	if req.PageToken != "" {
		offset, err = strconv.Atoi(req.PageToken)
		if err != nil || offset < 0 {
			return nil, status.Error(codes.InvalidArgument, "invalid page_token")
		}
	}

	s.maybePurge(ctx)

	notifications, unreadCount, err := s.repo.List(ctx, userID, filter, pageSize, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list notifications: %v", err)
	}

	resp := &pb.ListNotificationsResponse{
		Notifications: make([]*pb.Notification, 0, len(notifications)),
		UnreadCount:   int32(unreadCount),
	}
	for _, n := range notifications {
		resp.Notifications = append(resp.Notifications, notificationToProto(n))
	}
	if len(notifications) == pageSize {
		resp.NextPageToken = strconv.Itoa(offset + pageSize)
	}
	return resp, nil
}

// GetUnreadNotificationCount returns the caller's unread badge count.
func (s *NotificationService) GetUnreadNotificationCount(ctx context.Context, _ *pb.GetUnreadNotificationCountRequest) (*pb.GetUnreadNotificationCountResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountUnread(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count unread notifications: %v", err)
	}
	return &pb.GetUnreadNotificationCountResponse{Count: int32(count)}, nil
}

// MarkNotificationRead marks a single notification read (idempotent).
// Unknown or foreign ids are indistinguishable 404s.
func (s *NotificationService) MarkNotificationRead(ctx context.Context, req *pb.MarkNotificationReadRequest) (*pb.MarkNotificationReadResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	found, err := s.repo.MarkRead(ctx, userID, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mark notification read: %v", err)
	}
	if !found {
		return nil, status.Error(codes.NotFound, "notification not found")
	}
	return &pb.MarkNotificationReadResponse{Success: true}, nil
}

// MarkAllNotificationsRead marks every unread notification of the caller read.
func (s *NotificationService) MarkAllNotificationsRead(ctx context.Context, _ *pb.MarkAllNotificationsReadRequest) (*pb.MarkAllNotificationsReadResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.MarkAllRead(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mark all notifications read: %v", err)
	}
	return &pb.MarkAllNotificationsReadResponse{
		Success:      true,
		UpdatedCount: int32(updated),
	}, nil
}

// DeleteNotification removes a single notification owned by the caller.
func (s *NotificationService) DeleteNotification(ctx context.Context, req *pb.DeleteNotificationRequest) (*pb.DeleteNotificationResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	found, err := s.repo.Delete(ctx, userID, req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete notification: %v", err)
	}
	if !found {
		return nil, status.Error(codes.NotFound, "notification not found")
	}
	return &pb.DeleteNotificationResponse{Success: true}, nil
}

func notificationToProto(n *models.Notification) *pb.Notification {
	p := &pb.Notification{
		Id:               n.ID,
		Type:             n.Type,
		ActorId:          n.ActorID,
		ActorDisplayName: n.ActorDisplayName,
		ActorAvatar:      n.ActorAvatar.String,
		TemplateId:       n.TemplateID,
		TemplateTitle:    n.TemplateTitle,
		CreatedAt:        n.CreatedAt.Format(time.RFC3339),
		Snippet:          n.Snippet,
	}
	if n.ReadAt != nil {
		p.ReadAt = n.ReadAt.Format(time.RFC3339)
	}
	return p
}
