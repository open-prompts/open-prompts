package service

import (
	"context"
	"errors"
	"testing"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MockNotificationPreferencesRepository is a mock implementation of
// repository.NotificationPreferencesRepository.
type MockNotificationPreferencesRepository struct {
	mock.Mock
}

func (m *MockNotificationPreferencesRepository) Get(ctx context.Context, userID string) (*models.NotificationPreferences, bool, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Get(1).(bool), args.Error(2)
	}
	return args.Get(0).(*models.NotificationPreferences), args.Get(1).(bool), args.Error(2)
}

func (m *MockNotificationPreferencesRepository) Upsert(ctx context.Context, userID string, like, favorite, fork, comment, follow, paused *bool) error {
	args := m.Called(ctx, userID, like, favorite, fork, comment, follow, paused)
	return args.Error(0)
}

func newPrefsTestService(t *testing.T, prefsRepo repository.NotificationPreferencesRepository) *UserService {
	t.Helper()
	// nil Redis → the PATCH rate limit fails open (no mock expectations needed).
	svc := NewUserService(new(MockUserRepository), nil, nil, new(MockEmailService), "secret")
	svc.SetNotificationPrefsRepo(prefsRepo)
	return svc
}

func TestGetNotificationPreferences(t *testing.T) {
	t.Run("AbsentRowReturnsDefaults", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "user_1").Return(&models.NotificationPreferences{UserID: "user_1"}, false, nil).Once()
		svc := newPrefsTestService(t, prefsRepo)

		resp, err := svc.GetNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.GetNotificationPreferencesRequest{},
		)
		assert.NoError(t, err)
		assert.True(t, resp.LikeEnabled)
		assert.True(t, resp.FavoriteEnabled)
		assert.True(t, resp.ForkEnabled)
		assert.True(t, resp.CommentEnabled)
		assert.True(t, resp.FollowEnabled)
		assert.False(t, resp.PausedAll)
		prefsRepo.AssertExpectations(t)
	})

	t.Run("ExistingRowReturned", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "user_1").Return(&models.NotificationPreferences{
			UserID: "user_1", LikeEnabled: false, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: true, FollowEnabled: true, PausedAll: false,
		}, true, nil).Once()
		svc := newPrefsTestService(t, prefsRepo)

		resp, err := svc.GetNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.GetNotificationPreferencesRequest{},
		)
		assert.NoError(t, err)
		assert.False(t, resp.LikeEnabled)
		assert.True(t, resp.FavoriteEnabled)
		prefsRepo.AssertExpectations(t)
	})

	t.Run("RepoErrorReturnsInternal", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "user_1").Return(nil, false, errors.New("db down")).Once()
		svc := newPrefsTestService(t, prefsRepo)

		_, err := svc.GetNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.GetNotificationPreferencesRequest{},
		)
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		svc := newPrefsTestService(t, new(MockNotificationPreferencesRepository))
		_, err := svc.GetNotificationPreferences(context.Background(), &pb.GetNotificationPreferencesRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NilRepoReturnsInternal", func(t *testing.T) {
		svc := newPrefsTestService(t, nil)
		_, err := svc.GetNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.GetNotificationPreferencesRequest{},
		)
		assert.Equal(t, codes.Internal, status.Code(err))
	})
}

func TestUpdateNotificationPreferences(t *testing.T) {
	t.Run("PartialUpdateOnlyProvidedFields", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		like := false
		prefsRepo.On("Upsert", mock.Anything, "user_1",
			mock.MatchedBy(func(p *bool) bool { return p != nil && *p == false }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
		).Return(nil).Once()
		prefsRepo.On("Get", mock.Anything, "user_1").Return(&models.NotificationPreferences{
			UserID: "user_1", LikeEnabled: false, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: true, FollowEnabled: true, PausedAll: false,
		}, true, nil).Once()
		svc := newPrefsTestService(t, prefsRepo)

		resp, err := svc.UpdateNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.UpdateNotificationPreferencesRequest{LikeEnabled: &like},
		)
		assert.NoError(t, err)
		assert.False(t, resp.LikeEnabled)
		assert.True(t, resp.FavoriteEnabled)
		prefsRepo.AssertExpectations(t)
	})

	t.Run("PausedAllUpdate", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		paused := true
		prefsRepo.On("Upsert", mock.Anything, "user_1",
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p != nil && *p == true }),
		).Return(nil).Once()
		prefsRepo.On("Get", mock.Anything, "user_1").Return(&models.NotificationPreferences{
			UserID: "user_1", LikeEnabled: true, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: true, FollowEnabled: true, PausedAll: true,
		}, true, nil).Once()
		svc := newPrefsTestService(t, prefsRepo)

		resp, err := svc.UpdateNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.UpdateNotificationPreferencesRequest{PausedAll: &paused},
		)
		assert.NoError(t, err)
		assert.True(t, resp.PausedAll)
		prefsRepo.AssertExpectations(t)
	})

	t.Run("RateLimitExceeded", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		redis := new(MockRedisStore)
		// First call returns count >= max → rate limited.
		redis.On("Get", mock.Anything, "notif_prefs_rate_min:user_1").Return("30:9999999999", nil).Once()
		svc := NewUserService(new(MockUserRepository), nil, redis, new(MockEmailService), "secret")
		svc.SetNotificationPrefsRepo(prefsRepo)

		like := false
		_, err := svc.UpdateNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.UpdateNotificationPreferencesRequest{LikeEnabled: &like},
		)
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
		prefsRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("RateLimitFailOpenWhenRedisNil", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		like := false
		prefsRepo.On("Upsert", mock.Anything, "user_1",
			mock.MatchedBy(func(p *bool) bool { return p != nil && *p == false }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
			mock.MatchedBy(func(p *bool) bool { return p == nil }),
		).Return(nil).Once()
		prefsRepo.On("Get", mock.Anything, "user_1").Return(&models.NotificationPreferences{
			UserID: "user_1", LikeEnabled: false, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: true, FollowEnabled: true, PausedAll: false,
		}, true, nil).Once()
		svc := NewUserService(new(MockUserRepository), nil, nil, new(MockEmailService), "secret")
		svc.SetNotificationPrefsRepo(prefsRepo)

		resp, err := svc.UpdateNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.UpdateNotificationPreferencesRequest{LikeEnabled: &like},
		)
		assert.NoError(t, err)
		assert.False(t, resp.LikeEnabled)
		prefsRepo.AssertExpectations(t)
	})

	t.Run("UpsertErrorReturnsInternal", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		like := false
		prefsRepo.On("Upsert", mock.Anything, "user_1",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(errors.New("db down")).Once()
		svc := newPrefsTestService(t, prefsRepo)

		_, err := svc.UpdateNotificationPreferences(
			ContextWithUserID(context.Background(), "user_1"),
			&pb.UpdateNotificationPreferencesRequest{LikeEnabled: &like},
		)
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		svc := newPrefsTestService(t, new(MockNotificationPreferencesRepository))
		_, err := svc.UpdateNotificationPreferences(context.Background(), &pb.UpdateNotificationPreferencesRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

func TestPrefsAllow(t *testing.T) {
	t.Run("NilRepoAllows", func(t *testing.T) {
		assert.True(t, prefsAllow(context.Background(), nil, "user_1", "like"))
	})

	t.Run("AbsentRowAllows", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_absent").Return(&models.NotificationPreferences{UserID: "pa_absent"}, false, nil).Once()
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_absent", "like"))
	})

	t.Run("PausedAllSuppressesAll", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_paused").Return(&models.NotificationPreferences{
			UserID: "pa_paused", LikeEnabled: true, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: true, FollowEnabled: true, PausedAll: true,
		}, true, nil).Once()
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_paused", "like"))
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_paused", "follow"))
	})

	t.Run("DisabledTypeSuppresses", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_disabled").Return(&models.NotificationPreferences{
			UserID: "pa_disabled", LikeEnabled: false, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: true, FollowEnabled: true, PausedAll: false,
		}, true, nil).Once()
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_disabled", "like"))
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_disabled", "favorite"))
	})

	t.Run("CommentEnabledGovernsCommentAndCommentReply", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_comment").Return(&models.NotificationPreferences{
			UserID: "pa_comment", LikeEnabled: true, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: false, FollowEnabled: true, PausedAll: false,
		}, true, nil).Once()
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_comment", "comment"))
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_comment", "comment_reply"))
	})

	t.Run("AdminTypesNotSuppressed", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_admin").Return(&models.NotificationPreferences{
			UserID: "pa_admin", LikeEnabled: false, FavoriteEnabled: false, ForkEnabled: false,
			CommentEnabled: false, FollowEnabled: false, PausedAll: true,
		}, true, nil).Once()
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_admin", "moderation"))
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_admin", "appeal_resolved"))
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_admin", "template_approved"))
	})

	t.Run("RepoErrorFailsOpen", func(t *testing.T) {
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_err").Return(nil, false, errors.New("db down")).Once()
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_err", "like"))
	})

	t.Run("EachTypeToggleRespected", func(t *testing.T) {
		// favorite/fork/follow enabled, comment disabled, not paused.
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_types").Return(&models.NotificationPreferences{
			UserID: "pa_types", LikeEnabled: true, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: false, FollowEnabled: true, PausedAll: false,
		}, true, nil).Once()
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_types", "favorite"))
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_types", "fork"))
		assert.True(t, prefsAllow(context.Background(), prefsRepo, "pa_types", "follow"))
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_types", "comment"))
	})

	t.Run("PausedSuppressesEachType", func(t *testing.T) {
		// paused_all on suppresses every user type regardless of its toggle.
		prefsRepo := new(MockNotificationPreferencesRepository)
		prefsRepo.On("Get", mock.Anything, "pa_paused_types").Return(&models.NotificationPreferences{
			UserID: "pa_paused_types", LikeEnabled: true, FavoriteEnabled: true, ForkEnabled: true,
			CommentEnabled: true, FollowEnabled: true, PausedAll: true,
		}, true, nil).Once()
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_paused_types", "favorite"))
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_paused_types", "fork"))
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_paused_types", "comment"))
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_paused_types", "comment_reply"))
		assert.False(t, prefsAllow(context.Background(), prefsRepo, "pa_paused_types", "follow"))
	})
}

func TestPrefsCacheInvalidate(t *testing.T) {
	// A PATCH must invalidate the in-process cache so the change takes effect
	// immediately (FR-5). Verify the cache entry is removed.
	notifPrefsCache.set("user_1", &models.NotificationPreferences{UserID: "user_1"}, true)
	notifPrefsCache.invalidate("user_1")
	_, _, ok := notifPrefsCache.get("user_1")
	assert.False(t, ok)
}