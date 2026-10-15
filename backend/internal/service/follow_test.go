package service

import (
	"context"
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

// MockFollowRepository is a mock implementation of repository.FollowRepository.
type MockFollowRepository struct {
	mock.Mock
}

func (m *MockFollowRepository) Follow(ctx context.Context, followerID, followeeID string) (int32, bool, error) {
	args := m.Called(ctx, followerID, followeeID)
	return args.Get(0).(int32), args.Bool(1), args.Error(2)
}

func (m *MockFollowRepository) Unfollow(ctx context.Context, followerID, followeeID string) (int32, bool, error) {
	args := m.Called(ctx, followerID, followeeID)
	return args.Get(0).(int32), args.Bool(1), args.Error(2)
}

func (m *MockFollowRepository) IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error) {
	args := m.Called(ctx, followerID, followeeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockFollowRepository) GetFollowCounts(ctx context.Context, userID string) (int32, int32, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int32), args.Get(1).(int32), args.Error(2)
}

func (m *MockFollowRepository) ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error) {
	args := m.Called(ctx, userID, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]*models.UserCard), args.Int(1), args.Error(2)
}

func (m *MockFollowRepository) ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error) {
	args := m.Called(ctx, userID, page, pageSize)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]*models.UserCard), args.Int(1), args.Error(2)
}

func (m *MockFollowRepository) GetFollowingFeed(ctx context.Context, userID string, pageSize int, beforeCreatedAt time.Time) ([]*models.Template, int, error) {
	args := m.Called(ctx, userID, pageSize, beforeCreatedAt)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]*models.Template), args.Int(1), args.Error(2)
}

func (m *MockFollowRepository) TopCreatorsByFollowers(ctx context.Context, limit int) ([]*models.UserCard, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.UserCard), args.Error(1)
}

var _ repository.FollowRepository = (*MockFollowRepository)(nil)

func newFollowTestService(t *testing.T) (*UserService, *MockUserRepository, *MockFollowRepository, *MockNotificationRepository, *MockRedisStore) {
	userRepo := new(MockUserRepository)
	followRepo := new(MockFollowRepository)
	notifRepo := new(MockNotificationRepository)
	redis := new(MockRedisStore)
	svc := NewUserService(userRepo, nil, redis, nil, "secret")
	svc.SetFollowRepo(followRepo)
	svc.SetNotificationRepo(notifRepo)
	return svc, userRepo, followRepo, notifRepo, redis
}

func TestFollowUser(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, userRepo, followRepo, notifRepo, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active", DisplayName: "Alice"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		followRepo.On("Follow", mock.Anything, "alice", "bob").Return(int32(1), true, nil)
		notifRepo.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.RecipientID == "bob" && n.ActorID == "alice" && n.Type == "follow"
		})).Return(nil)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)

		resp, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.NoError(t, err)
		assert.True(t, resp.Following)
		assert.Equal(t, int32(1), resp.FollowerCount)
		notifRepo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("SelfFollowRejected", func(t *testing.T) {
		svc, _, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "alice"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("FolloweeNotFound", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "ghost").Return(nil, repository.ErrUserNotFound)
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "ghost"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("BannedFollowerRejected", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "banned"}, nil)
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("BannedFolloweeRejected", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "banned"}, nil)
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("RateLimited", func(t *testing.T) {
		svc, userRepo, followRepo, _, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		// Redis returns an over-limit counter.
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("30:9999999999", nil)
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
		assert.Contains(t, err.Error(), "retry after")
		followRepo.AssertNotCalled(t, "Follow", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("IdempotentNoNotification", func(t *testing.T) {
		svc, userRepo, followRepo, notifRepo, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		// inserted=false: already following, no notification.
		followRepo.On("Follow", mock.Anything, "alice", "bob").Return(int32(1), false, nil)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)

		resp, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.NoError(t, err)
		assert.True(t, resp.Following)
		notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, userRepo, followRepo, _, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		followRepo.On("Follow", mock.Anything, "alice", "bob").Return(int32(0), false, assert.AnError)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("FollowerLoadError", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(nil, assert.AnError)
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("FolloweeLoadError", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(nil, assert.AnError)
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("MissingUserID", func(t *testing.T) {
		svc, _, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		_, err := svc.FollowUser(ctx, &pb.FollowUserRequest{UserId: ""})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestUnfollowUser(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, userRepo, followRepo, _, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		followRepo.On("Unfollow", mock.Anything, "alice", "bob").Return(int32(0), true, nil)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)

		resp, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.NoError(t, err)
		assert.False(t, resp.Following)
		assert.Equal(t, int32(0), resp.FollowerCount)
	})

	t.Run("SelfUnfollowRejected", func(t *testing.T) {
		svc, _, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "alice"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("FolloweeNotFound", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "ghost").Return(nil, repository.ErrUserNotFound)
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "ghost"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("BannedFollowerRejected", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "banned"}, nil)
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("RateLimited", func(t *testing.T) {
		svc, userRepo, followRepo, _, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("30:9999999999", nil)
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
		followRepo.AssertNotCalled(t, "Unfollow", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Idempotent", func(t *testing.T) {
		svc, userRepo, followRepo, _, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		followRepo.On("Unfollow", mock.Anything, "alice", "bob").Return(int32(0), false, nil)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)
		resp, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.NoError(t, err)
		assert.False(t, resp.Following)
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, userRepo, followRepo, _, redis := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", Status: "active"}, nil)
		followRepo.On("Unfollow", mock.Anything, "alice", "bob").Return(int32(0), false, assert.AnError)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("MissingUserID", func(t *testing.T) {
		svc, _, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: ""})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("FollowerLoadError", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(nil, assert.AnError)
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("FolloweeLoadError", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Status: "active"}, nil)
		userRepo.On("GetByID", mock.Anything, "bob").Return(nil, assert.AnError)
		_, err := svc.UnfollowUser(ctx, &pb.FollowUserRequest{UserId: "bob"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})
}

func TestGetCreatorProfile(t *testing.T) {
	t.Run("AnonymousNoFollowFlags", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{
			ID: "bob", DisplayName: "Bob", Avatar: "a", Bio: "hi",
			FollowerCount: 5, FollowingCount: 2, CreatedAt: time.Now(),
		}, nil)
		resp, err := svc.GetCreatorProfile(context.Background(), &pb.GetCreatorProfileRequest{UserId: "bob"})
		assert.NoError(t, err)
		assert.Equal(t, "bob", resp.Id)
		assert.Equal(t, "hi", resp.Bio)
		assert.Equal(t, int32(5), resp.FollowerCount)
		assert.False(t, resp.IsFollowing)
		assert.False(t, resp.FollowsYou)
	})

	t.Run("AuthedComputesFlags", func(t *testing.T) {
		svc, userRepo, followRepo, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob", DisplayName: "Bob"}, nil)
		followRepo.On("IsFollowing", mock.Anything, "alice", "bob").Return(true, nil)
		followRepo.On("IsFollowing", mock.Anything, "bob", "alice").Return(false, nil)
		resp, err := svc.GetCreatorProfile(context.Background(), &pb.GetCreatorProfileRequest{UserId: "bob", ViewerId: "alice"})
		assert.NoError(t, err)
		assert.True(t, resp.IsFollowing)
		assert.False(t, resp.FollowsYou)
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "ghost").Return(nil, repository.ErrUserNotFound)
		_, err := svc.GetCreatorProfile(context.Background(), &pb.GetCreatorProfileRequest{UserId: "ghost"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

func TestListFollowersFollowing(t *testing.T) {
	t.Run("ListFollowers", func(t *testing.T) {
		svc, userRepo, followRepo, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob"}, nil)
		cards := []*models.UserCard{{ID: "alice", DisplayName: "Alice", FollowerCount: 3}}
		followRepo.On("ListFollowers", mock.Anything, "bob", 1, 20).Return(cards, 1, nil)
		followRepo.On("IsFollowing", mock.Anything, "", "alice").Return(false, nil)
		resp, err := svc.ListFollowers(context.Background(), &pb.ListUserRelationsRequest{UserId: "bob"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(resp.Users))
		assert.Equal(t, "alice", resp.Users[0].Id)
		assert.Equal(t, int32(1), resp.TotalCount)
	})

	t.Run("ListFollowing", func(t *testing.T) {
		svc, userRepo, followRepo, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice"}, nil)
		cards := []*models.UserCard{{ID: "bob", DisplayName: "Bob"}}
		followRepo.On("ListFollowing", mock.Anything, "alice", 1, 20).Return(cards, 1, nil)
		followRepo.On("IsFollowing", mock.Anything, "", "bob").Return(false, nil)
		resp, err := svc.ListFollowing(context.Background(), &pb.ListUserRelationsRequest{UserId: "alice"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(resp.Users))
		assert.Equal(t, "bob", resp.Users[0].Id)
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "ghost").Return(nil, repository.ErrUserNotFound)
		_, err := svc.ListFollowers(context.Background(), &pb.ListUserRelationsRequest{UserId: "ghost"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, userRepo, followRepo, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob"}, nil)
		followRepo.On("ListFollowers", mock.Anything, "bob", 1, 20).Return(nil, 0, assert.AnError)
		_, err := svc.ListFollowers(context.Background(), &pb.ListUserRelationsRequest{UserId: "bob"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("MissingUserID", func(t *testing.T) {
		svc, _, _, _, _ := newFollowTestService(t)
		_, err := svc.ListFollowers(context.Background(), &pb.ListUserRelationsRequest{UserId: ""})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("PageSizeCapped", func(t *testing.T) {
		svc, userRepo, followRepo, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "bob").Return(&models.User{ID: "bob"}, nil)
		followRepo.On("ListFollowers", mock.Anything, "bob", 1, 50).Return([]*models.UserCard{}, 0, nil)
		resp, err := svc.ListFollowers(context.Background(), &pb.ListUserRelationsRequest{UserId: "bob", PageSize: 100})
		assert.NoError(t, err)
		assert.Equal(t, int32(50), resp.PageSize)
	})
}

func TestGetFollowingFeed(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, _, followRepo, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		tpl := &models.Template{ID: "t1", OwnerID: "bob", Title: "T", Visibility: "public", OwnerDisplayName: "Bob"}
		followRepo.On("GetFollowingFeed", mock.Anything, "alice", 20, mock.Anything).Return([]*models.Template{tpl}, 1, nil)
		resp, err := svc.GetFollowingFeed(ctx, &pb.GetFollowingFeedRequest{})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(resp.Templates))
		assert.Equal(t, "t1", resp.Templates[0].Id)
		assert.Equal(t, "Bob", resp.Templates[0].OwnerDisplayName)
	})

	t.Run("InvalidPageToken", func(t *testing.T) {
		svc, _, _, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		_, err := svc.GetFollowingFeed(ctx, &pb.GetFollowingFeedRequest{PageToken: "not-a-number"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("RepoError", func(t *testing.T) {
		svc, _, followRepo, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		followRepo.On("GetFollowingFeed", mock.Anything, "alice", 20, mock.Anything).Return(nil, 0, assert.AnError)
		_, err := svc.GetFollowingFeed(ctx, &pb.GetFollowingFeedRequest{})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("NextPageToken", func(t *testing.T) {
		svc, _, followRepo, _, _ := newFollowTestService(t)
		ctx := ContextWithUserID(context.Background(), "alice")
		now := time.Now()
		tpls := make([]*models.Template, 20)
		for i := range tpls {
			tpls[i] = &models.Template{ID: "t", OwnerID: "bob", Visibility: "public", CreatedAt: now.Add(-time.Duration(i) * time.Second)}
		}
		followRepo.On("GetFollowingFeed", mock.Anything, "alice", 20, mock.Anything).Return(tpls, 20, nil)
		resp, err := svc.GetFollowingFeed(ctx, &pb.GetFollowingFeedRequest{})
		assert.NoError(t, err)
		assert.NotEmpty(t, resp.NextPageToken)
	})
}

func TestListTopCreators(t *testing.T) {
	svc, _, followRepo, _, _ := newFollowTestService(t)
	cards := []*models.UserCard{{ID: "bob", DisplayName: "Bob", FollowerCount: 10}}
	followRepo.On("TopCreatorsByFollowers", mock.Anything, 5).Return(cards, nil)
	followRepo.On("IsFollowing", mock.Anything, "", "bob").Return(false, nil)
	resp, err := svc.ListTopCreators(context.Background(), &pb.ListTopCreatorsRequest{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(resp.Users))
	assert.Equal(t, "bob", resp.Users[0].Id)
}

func TestListTopCreatorsErrors(t *testing.T) {
	t.Run("RepoError", func(t *testing.T) {
		svc, _, followRepo, _, _ := newFollowTestService(t)
		followRepo.On("TopCreatorsByFollowers", mock.Anything, 5).Return(nil, assert.AnError)
		_, err := svc.ListTopCreators(context.Background(), &pb.ListTopCreatorsRequest{})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("LimitCapped", func(t *testing.T) {
		svc, _, followRepo, _, _ := newFollowTestService(t)
		followRepo.On("TopCreatorsByFollowers", mock.Anything, 20).Return([]*models.UserCard{}, nil)
		_, err := svc.ListTopCreators(context.Background(), &pb.ListTopCreatorsRequest{Limit: 100})
		assert.NoError(t, err)
	})
}

func TestCheckFollowRate(t *testing.T) {
	t.Run("FailOpenWhenRedisNil", func(t *testing.T) {
		svc := NewUserService(new(MockUserRepository), nil, nil, nil, "secret")
		_, ok := svc.checkFollowRate(context.Background(), "alice")
		assert.True(t, ok)
	})

	t.Run("WindowElapsedResets", func(t *testing.T) {
		svc, _, _, _, redis := newFollowTestService(t)
		// Counter from an expired window.
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("30:1", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)
		_, ok := svc.checkFollowRate(context.Background(), "alice")
		assert.True(t, ok)
	})

	t.Run("MalformedValueResets", func(t *testing.T) {
		svc, _, _, _, redis := newFollowTestService(t)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("garbage", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)
		_, ok := svc.checkFollowRate(context.Background(), "alice")
		assert.True(t, ok)
	})

	t.Run("UnderLimitIncrements", func(t *testing.T) {
		svc, _, _, _, redis := newFollowTestService(t)
		redis.On("Get", mock.Anything, "follow_rate_min:alice").Return("5:9999999999", nil)
		redis.On("Set", mock.Anything, "follow_rate_min:alice", mock.Anything, mock.Anything).Return(nil)
		_, ok := svc.checkFollowRate(context.Background(), "alice")
		assert.True(t, ok)
	})
}

func TestEmitFollowNotification(t *testing.T) {
	t.Run("NilReposSkipped", func(t *testing.T) {
		svc := NewUserService(new(MockUserRepository), nil, nil, nil, "secret")
		svc.emitFollowNotification(context.Background(), "bob", "alice") // no panic
	})

	t.Run("ActorLoadFailureLogged", func(t *testing.T) {
		svc, userRepo, _, notifRepo, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "alice").Return(nil, repository.ErrUserNotFound)
		svc.emitFollowNotification(context.Background(), "bob", "alice")
		notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("CreateFailureLogged", func(t *testing.T) {
		svc, userRepo, _, notifRepo, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", DisplayName: "Alice"}, nil)
		notifRepo.On("Create", mock.Anything, mock.Anything).Return(assert.AnError)
		svc.emitFollowNotification(context.Background(), "bob", "alice") // no panic
	})
}

func TestUpdateProfileBio(t *testing.T) {
	t.Run("SetsBio", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice"}, nil)
		userRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
			return u.Bio == "hello world"
		})).Return(nil)
		resp, err := svc.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{Id: "alice", Bio: "  hello world  "})
		assert.NoError(t, err)
		assert.Equal(t, "hello world", resp.Bio)
	})

	t.Run("ClearsBio", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", Bio: "old"}, nil)
		userRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
			return u.Bio == ""
		})).Return(nil)
		resp, err := svc.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{Id: "alice", Bio: ""})
		assert.NoError(t, err)
		assert.Equal(t, "", resp.Bio)
	})

	t.Run("TooLong", func(t *testing.T) {
		svc, userRepo, _, _, _ := newFollowTestService(t)
		userRepo.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice"}, nil)
		long := make([]rune, 281)
		for i := range long {
			long[i] = 'a'
		}
		_, err := svc.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{Id: "alice", Bio: string(long)})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}