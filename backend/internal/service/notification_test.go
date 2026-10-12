package service

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---- mocks ----

// MockNotificationRepository is a mock implementation of repository.NotificationRepository.
type MockNotificationRepository struct {
	mock.Mock
}

func (m *MockNotificationRepository) Create(ctx context.Context, n *models.Notification) error {
	args := m.Called(ctx, n)
	return args.Error(0)
}

func (m *MockNotificationRepository) List(ctx context.Context, recipientID, filter string, limit, offset int) ([]*models.Notification, int, error) {
	args := m.Called(ctx, recipientID, filter, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int), args.Error(2)
	}
	return args.Get(0).([]*models.Notification), args.Get(1).(int), args.Error(2)
}

func (m *MockNotificationRepository) CountUnread(ctx context.Context, recipientID string) (int, error) {
	args := m.Called(ctx, recipientID)
	return args.Get(0).(int), args.Error(1)
}

func (m *MockNotificationRepository) MarkRead(ctx context.Context, recipientID, id string) (bool, error) {
	args := m.Called(ctx, recipientID, id)
	return args.Get(0).(bool), args.Error(1)
}

func (m *MockNotificationRepository) MarkAllRead(ctx context.Context, recipientID string) (int64, error) {
	args := m.Called(ctx, recipientID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockNotificationRepository) Delete(ctx context.Context, recipientID, id string) (bool, error) {
	args := m.Called(ctx, recipientID, id)
	return args.Get(0).(bool), args.Error(1)
}

func (m *MockNotificationRepository) PurgeExpired(ctx context.Context, readRetentionDays, retentionDays int) error {
	args := m.Called(ctx, readRetentionDays, retentionDays)
	return args.Error(0)
}

func newNotificationTestService(t *testing.T) (*NotificationService, *MockNotificationRepository) {
	t.Helper()
	repo := new(MockNotificationRepository)
	svc := NewNotificationService(repo, 90, 180)
	return svc, repo
}

func TestNewNotificationServiceDefaults(t *testing.T) {
	repo := new(MockNotificationRepository)
	svc := NewNotificationService(repo, 0, -5)
	assert.Equal(t, defaultNotificationReadRetentionDays, svc.readRetentionDays)
	assert.Equal(t, defaultNotificationRetentionDays, svc.retentionDays)

	svc2 := NewNotificationService(repo, 30, 60)
	assert.Equal(t, 30, svc2.readRetentionDays)
	assert.Equal(t, 60, svc2.retentionDays)
}

func TestListNotifications(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	readAt := now.Add(-time.Hour)

	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.ListNotifications(context.Background(), &pb.ListNotificationsRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("DefaultFilterAndPaging", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("List", mock.Anything, "user_1", "all", 20, 0).
			Return([]*models.Notification{
				{
					ID:               "n1",
					Type:             "like",
					ActorID:          "actor_1",
					ActorDisplayName: "Actor One",
					TemplateID:       "t1",
					TemplateTitle:    "My Template",
					CreatedAt:        now,
					ReadAt:           &readAt,
				},
			}, 3, nil).Once()
		repo.On("PurgeExpired", mock.Anything, 90, 180).Return(nil).Once()

		resp, err := svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{},
		)
		assert.NoError(t, err)
		assert.Len(t, resp.Notifications, 1)
		assert.Equal(t, int32(3), resp.UnreadCount)
		assert.Empty(t, resp.NextPageToken, "no next page when fewer items than page size")

		n := resp.Notifications[0]
		assert.Equal(t, "n1", n.Id)
		assert.Equal(t, "like", n.Type)
		assert.Equal(t, "actor_1", n.ActorId)
		assert.Equal(t, "Actor One", n.ActorDisplayName)
		assert.Equal(t, "t1", n.TemplateId)
		assert.Equal(t, "My Template", n.TemplateTitle)
		assert.Equal(t, now.Format(time.RFC3339), n.CreatedAt)
		assert.Equal(t, readAt.Format(time.RFC3339), n.ReadAt)
		repo.AssertExpectations(t)
	})

	t.Run("UnreadFilterAndNextPageToken", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		full := make([]*models.Notification, 20)
		for i := range full {
			full[i] = &models.Notification{ID: "n", Type: "fork", CreatedAt: now}
		}
		repo.On("List", mock.Anything, "user_1", "unread", 20, 20).
			Return(full, 20, nil).Once()
		repo.On("PurgeExpired", mock.Anything, 90, 180).Return(nil).Once()

		resp, err := svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{Filter: "unread", PageToken: "20"},
		)
		assert.NoError(t, err)
		assert.Len(t, resp.Notifications, 20)
		assert.Equal(t, "40", resp.NextPageToken)
		repo.AssertExpectations(t)
	})

	t.Run("InvalidFilter", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{Filter: "read"},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("InvalidPageSize", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{PageSize: 101},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{PageSize: -1},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("InvalidPageToken", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{PageToken: "abc"},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{PageToken: "-3"},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("List", mock.Anything, "user_1", "all", 20, 0).
			Return(nil, 0, errors.New("db down")).Once()
		repo.On("PurgeExpired", mock.Anything, 90, 180).Return(nil).Once()

		_, err := svc.ListNotifications(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.ListNotificationsRequest{},
		)
		assert.Equal(t, codes.Internal, status.Code(err))
		repo.AssertExpectations(t)
	})
}

func TestGetUnreadNotificationCount(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.GetUnreadNotificationCount(context.Background(), &pb.GetUnreadNotificationCountRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("CountUnread", mock.Anything, "user_1").Return(7, nil).Once()

		resp, err := svc.GetUnreadNotificationCount(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.GetUnreadNotificationCountRequest{},
		)
		assert.NoError(t, err)
		assert.Equal(t, int32(7), resp.Count)
		repo.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("CountUnread", mock.Anything, "user_1").Return(0, errors.New("db down")).Once()

		_, err := svc.GetUnreadNotificationCount(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.GetUnreadNotificationCountRequest{},
		)
		assert.Equal(t, codes.Internal, status.Code(err))
		repo.AssertExpectations(t)
	})
}

func TestMarkNotificationRead(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.MarkNotificationRead(context.Background(), &pb.MarkNotificationReadRequest{Id: "n1"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("MissingId", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.MarkNotificationRead(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.MarkNotificationReadRequest{},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("MarkRead", mock.Anything, "user_1", "n1").Return(true, nil).Once()

		resp, err := svc.MarkNotificationRead(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.MarkNotificationReadRequest{Id: "n1"},
		)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
		repo.AssertExpectations(t)
	})

	t.Run("NotFoundForForeignOrUnknownId", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("MarkRead", mock.Anything, "user_1", "other").Return(false, nil).Once()

		_, err := svc.MarkNotificationRead(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.MarkNotificationReadRequest{Id: "other"},
		)
		assert.Equal(t, codes.NotFound, status.Code(err))
		repo.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("MarkRead", mock.Anything, "user_1", "n1").Return(false, errors.New("db down")).Once()

		_, err := svc.MarkNotificationRead(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.MarkNotificationReadRequest{Id: "n1"},
		)
		assert.Equal(t, codes.Internal, status.Code(err))
		repo.AssertExpectations(t)
	})
}

func TestMarkAllNotificationsRead(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.MarkAllNotificationsRead(context.Background(), &pb.MarkAllNotificationsReadRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("MarkAllRead", mock.Anything, "user_1").Return(int64(5), nil).Once()

		resp, err := svc.MarkAllNotificationsRead(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.MarkAllNotificationsReadRequest{},
		)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, int32(5), resp.UpdatedCount)
		repo.AssertExpectations(t)
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("MarkAllRead", mock.Anything, "user_1").Return(int64(0), errors.New("db down")).Once()

		_, err := svc.MarkAllNotificationsRead(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.MarkAllNotificationsReadRequest{},
		)
		assert.Equal(t, codes.Internal, status.Code(err))
		repo.AssertExpectations(t)
	})
}

func TestDeleteNotification(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.DeleteNotification(context.Background(), &pb.DeleteNotificationRequest{Id: "n1"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("MissingId", func(t *testing.T) {
		svc, _ := newNotificationTestService(t)
		_, err := svc.DeleteNotification(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.DeleteNotificationRequest{},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("Delete", mock.Anything, "user_1", "n1").Return(true, nil).Once()

		resp, err := svc.DeleteNotification(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.DeleteNotificationRequest{Id: "n1"},
		)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
		repo.AssertExpectations(t)
	})

	t.Run("NotFoundForForeignOrUnknownId", func(t *testing.T) {
		svc, repo := newNotificationTestService(t)
		repo.On("Delete", mock.Anything, "user_1", "other").Return(false, nil).Once()

		_, err := svc.DeleteNotification(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.DeleteNotificationRequest{Id: "other"},
		)
		assert.Equal(t, codes.NotFound, status.Code(err))
		repo.AssertExpectations(t)
	})
}

func TestNotificationPurgeThrottle(t *testing.T) {
	svc, repo := newNotificationTestService(t)
	repo.On("PurgeExpired", mock.Anything, 90, 180).Return(nil).Once()
	repo.On("List", mock.Anything, "user_1", "all", 20, 0).Return(nil, 0, nil).Times(3)

	ctx := ContextWithUserID(context.Background(), "user_1")
	for i := 0; i < 3; i++ {
		_, err := svc.ListNotifications(ctx, &pb.ListNotificationsRequest{})
		assert.NoError(t, err)
	}
	// PurgeExpired must run only once despite three list calls.
	repo.AssertExpectations(t)
}

func TestNotificationPurgeErrorSwallowed(t *testing.T) {
	svc, repo := newNotificationTestService(t)
	repo.On("PurgeExpired", mock.Anything, 90, 180).Return(errors.New("purge failed")).Once()
	repo.On("List", mock.Anything, "user_1", "all", 20, 0).Return(nil, 0, nil).Once()

	resp, err := svc.ListNotifications(
		ContextWithUserID(context.Background(), "user_1"),
		&pb.ListNotificationsRequest{},
	)
	assert.NoError(t, err, "purge failure must not fail the list call")
	assert.NotNil(t, resp)
	repo.AssertExpectations(t)
}

func TestNotificationToProto(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	n := &models.Notification{
		ID:               "n1",
		Type:             "fork",
		ActorID:          "actor_1",
		ActorDisplayName: "Actor",
		TemplateID:       "t1",
		TemplateTitle:    "Title",
		CreatedAt:        now,
	}
	p := notificationToProto(n)
	assert.Equal(t, "n1", p.Id)
	assert.Equal(t, "fork", p.Type)
	assert.Empty(t, p.ReadAt, "unread notification has empty read_at")

	readAt := now.Add(-time.Minute)
	n.ReadAt = &readAt
	p = notificationToProto(n)
	assert.Equal(t, readAt.Format(time.RFC3339), p.ReadAt)
}

// Compile-time interface check.
var _ repository.NotificationRepository = (*MockNotificationRepository)(nil)
