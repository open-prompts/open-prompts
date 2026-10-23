package service

import (
	"context"
	"errors"
	"testing"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ratingsTestTemplate returns a public template owned by "owner-1".
func ratingsTestTemplate() *models.Template {
	return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "public"}
}

// newRatingsTestService wires a PromptService with a mock template repo, a
// mock user repo and a fakeRedis for rate-limit tests.
func newRatingsTestService(t *testing.T) (*PromptService, *MockTemplateRepository, *MockUserRepository, *fakeRedis) {
	t.Helper()
	templateRepo := new(MockTemplateRepository)
	userRepo := new(MockUserRepository)
	versionRepo := new(MockTemplateVersionRepository)
	redis := newFakeRedis()
	svc := NewPromptService(nil, templateRepo, versionRepo, nil, nil, userRepo, nil, redis)
	return svc, templateRepo, userRepo, redis
}

func TestRateTemplate_Validation(t *testing.T) {
	svc, templateRepo, _, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}

	for _, rating := range []int32{0, -1, 6, 100} {
		_, err := svc.RateTemplate(
			ContextWithUserID(context.Background(), "rater-1"),
			&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: rating},
		)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), "rating %d should be invalid", rating)
	}
}

func TestRateTemplate_Unauthenticated(t *testing.T) {
	svc, _, _, _ := newRatingsTestService(t)
	_, err := svc.RateTemplate(context.Background(), &pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestRateTemplate_NotFound(t *testing.T) {
	svc, templateRepo, _, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return nil, errors.New("template not found")
	}
	_, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.RateTemplateRequest{TemplateId: "missing", Rating: 4},
	)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestRateTemplate_PrivateTemplate404(t *testing.T) {
	svc, templateRepo, _, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "private"}, nil
	}
	_, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4},
	)
	assert.Equal(t, codes.NotFound, status.Code(err), "private template should 404 for non-owner")
}

func TestDeleteRating_PrivateTemplate404(t *testing.T) {
	svc, templateRepo, _, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "private"}, nil
	}
	_, err := svc.DeleteRating(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.DeleteRatingRequest{TemplateId: "tpl-1"},
	)
	assert.Equal(t, codes.NotFound, status.Code(err), "private template should 404 for non-owner")
}

func TestRateTemplate_OwnerSelfRating(t *testing.T) {
	svc, templateRepo, _, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	_, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "owner-1"),
		&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4},
	)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRateTemplate_BannedCaller(t *testing.T) {
	svc, templateRepo, userRepo, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	userRepo.On("GetByID", mock.Anything, "rater-1").Return(&models.User{ID: "rater-1", Status: "banned"}, nil)

	_, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4},
	)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRateTemplate_Success(t *testing.T) {
	svc, templateRepo, userRepo, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	userRepo.On("GetByID", mock.Anything, "rater-1").Return(&models.User{ID: "rater-1", Status: "active"}, nil)
	templateRepo.RateTemplateFunc = func(ctx context.Context, userID, templateID string, rating int32) (int64, int32, error) {
		return 4, 1, nil
	}

	resp, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4},
	)
	require.NoError(t, err)
	assert.Equal(t, 4.0, resp.RatingAvg)
	assert.Equal(t, int32(1), resp.RatingCount)
	assert.Equal(t, int32(4), resp.MyRating)
}

func TestRateTemplate_RepoError(t *testing.T) {
	svc, templateRepo, userRepo, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	userRepo.On("GetByID", mock.Anything, "rater-1").Return(&models.User{ID: "rater-1", Status: "active"}, nil)
	templateRepo.RateTemplateFunc = func(ctx context.Context, userID, templateID string, rating int32) (int64, int32, error) {
		return 0, 0, errors.New("db down")
	}

	_, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4},
	)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestDeleteRating_Success(t *testing.T) {
	svc, templateRepo, _, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	templateRepo.DeleteRatingFunc = func(ctx context.Context, userID, templateID string) (int64, int32, error) {
		return 0, 0, nil
	}

	resp, err := svc.DeleteRating(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.DeleteRatingRequest{TemplateId: "tpl-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, 0.0, resp.RatingAvg)
	assert.Equal(t, int32(0), resp.RatingCount)
	assert.Equal(t, int32(0), resp.MyRating)
}

func TestDeleteRating_NotFound(t *testing.T) {
	svc, templateRepo, _, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return nil, errors.New("template not found")
	}
	_, err := svc.DeleteRating(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.DeleteRatingRequest{TemplateId: "missing"},
	)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestDeleteRating_Unauthenticated(t *testing.T) {
	svc, _, _, _ := newRatingsTestService(t)
	_, err := svc.DeleteRating(context.Background(), &pb.DeleteRatingRequest{TemplateId: "tpl-1"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestRateTemplate_RateLimit(t *testing.T) {
	svc, templateRepo, userRepo, redis := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	userRepo.On("GetByID", mock.Anything, "rater-1").Return(&models.User{ID: "rater-1", Status: "active"}, nil)
	templateRepo.RateTemplateFunc = func(ctx context.Context, userID, templateID string, rating int32) (int64, int32, error) {
		return int64(rating), 1, nil
	}

	// Seed the fixed-window counter at the limit (20) for this user.
	_ = redis.Set(context.Background(), "rating_rate_min:rater-1", "20:9999999999", 0)

	_, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4},
	)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestRateTemplate_RateLimitFailOpen(t *testing.T) {
	svc, templateRepo, userRepo, redis := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	userRepo.On("GetByID", mock.Anything, "rater-1").Return(&models.User{ID: "rater-1", Status: "active"}, nil)
	templateRepo.RateTemplateFunc = func(ctx context.Context, userID, templateID string, rating int32) (int64, int32, error) {
		return int64(rating), 1, nil
	}
	redis.fail = true // Redis down -> fail-open

	resp, err := svc.RateTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.RateTemplateRequest{TemplateId: "tpl-1", Rating: 4},
	)
	require.NoError(t, err, "rate limiting must fail open when Redis is down")
	assert.Equal(t, int32(4), resp.MyRating)
}

func TestRatingResponse_AverageRounding(t *testing.T) {
	// 3 + 5 = 8 over 2 ratings -> 4.0
	resp := ratingResponse(8, 2, 3)
	assert.Equal(t, 4.0, resp.RatingAvg)
	assert.Equal(t, int32(2), resp.RatingCount)
	assert.Equal(t, int32(3), resp.MyRating)

	// Zero count -> avg 0.
	zero := ratingResponse(0, 0, 0)
	assert.Equal(t, 0.0, zero.RatingAvg)
	assert.Equal(t, int32(0), zero.RatingCount)
}

func TestGetTemplate_RatingDistribution(t *testing.T) {
	svc, templateRepo, userRepo, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	templateRepo.GetRatingDistributionFunc = func(ctx context.Context, templateID string) ([5]int32, error) {
		return [5]int32{1, 0, 2, 0, 1}, nil
	}
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(&models.User{ID: "owner-1", DisplayName: "Owner"}, nil)

	resp, err := svc.GetTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.GetTemplateRequest{Id: "tpl-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 0, 2, 0, 1}, resp.RatingDistribution)
}

func TestGetTemplate_RatingDistributionErrorSwallowed(t *testing.T) {
	svc, templateRepo, userRepo, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return ratingsTestTemplate(), nil
	}
	templateRepo.GetRatingDistributionFunc = func(ctx context.Context, templateID string) ([5]int32, error) {
		return [5]int32{}, errors.New("db down")
	}
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(&models.User{ID: "owner-1", DisplayName: "Owner"}, nil)

	resp, err := svc.GetTemplate(
		ContextWithUserID(context.Background(), "rater-1"),
		&pb.GetTemplateRequest{Id: "tpl-1"},
	)
	require.NoError(t, err, "distribution failure must not fail the detail read")
	assert.Nil(t, resp.RatingDistribution)
}

func TestTopRatedMinRatings(t *testing.T) {
	svc, _, _, _ := newRatingsTestService(t)

	t.Setenv("TOP_RATED_MIN_RATINGS", "")
	assert.Equal(t, topRatedMinRatingsDef, svc.topRatedMinRatings(), "empty env falls back to default")

	t.Setenv("TOP_RATED_MIN_RATINGS", "3")
	assert.Equal(t, 3, svc.topRatedMinRatings())

	t.Setenv("TOP_RATED_MIN_RATINGS", "0")
	assert.Equal(t, topRatedMinRatingsDef, svc.topRatedMinRatings(), "non-positive falls back to default")

	t.Setenv("TOP_RATED_MIN_RATINGS", "abc")
	assert.Equal(t, topRatedMinRatingsDef, svc.topRatedMinRatings(), "invalid falls back to default")
}

func TestTemplateModelToProto_RatingMapping(t *testing.T) {
	svc, _, _, _ := newRatingsTestService(t)

	// Zero count -> avg 0.
	zero := svc.templateModelToProto(&models.Template{ID: "t1", RatingSum: 0, RatingCount: 0, MyRating: 0})
	assert.Equal(t, 0.0, zero.RatingAvg)
	assert.Equal(t, int32(0), zero.RatingCount)
	assert.Equal(t, int32(0), zero.MyRating)

	// sum/count computed at mapping time.
	rated := svc.templateModelToProto(&models.Template{ID: "t1", RatingSum: 14, RatingCount: 4, MyRating: 3})
	assert.Equal(t, 3.5, rated.RatingAvg)
	assert.Equal(t, int32(4), rated.RatingCount)
	assert.Equal(t, int32(3), rated.MyRating)
}

func TestTemplateModelToProto_ModerationReasonMapped(t *testing.T) {
	svc, _, _, _ := newRatingsTestService(t)

	tpl := svc.templateModelToProto(&models.Template{
		ID: "t1", OwnerID: "owner-1", ModerationStatus: "unpublished_by_admin",
		ModerationReason: "spam — off-platform links",
	})
	assert.Equal(t, "unpublished_by_admin", tpl.ModerationStatus)
	assert.Equal(t, "spam — off-platform links", tpl.ModerationReason)
}

func TestClearModerationReasonForNonOwner(t *testing.T) {
	// Owner keeps the reason.
	owner := &pb.Template{Id: "t1", OwnerId: "owner-1", ModerationReason: "policy violation"}
	clearModerationReasonForNonOwner(owner, "owner-1")
	assert.Equal(t, "policy violation", owner.ModerationReason)

	// Anonymous viewer gets it blanked.
	anon := &pb.Template{Id: "t1", OwnerId: "owner-1", ModerationReason: "policy violation"}
	clearModerationReasonForNonOwner(anon, "")
	assert.Equal(t, "", anon.ModerationReason)

	// Non-owner viewer gets it blanked.
	other := &pb.Template{Id: "t1", OwnerId: "owner-1", ModerationReason: "policy violation"}
	clearModerationReasonForNonOwner(other, "viewer-9")
	assert.Equal(t, "", other.ModerationReason)

	// Nil is a no-op.
	clearModerationReasonForNonOwner(nil, "owner-1")
}

func TestGetTemplate_ModerationReasonOwnerOnly(t *testing.T) {
	svc, templateRepo, userRepo, _ := newRatingsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{
			ID: "tpl-1", OwnerID: "owner-1", Visibility: "public",
			ModerationStatus: "unpublished_by_admin", ModerationReason: "policy violation",
		}, nil
	}
	templateRepo.GetRatingDistributionFunc = func(ctx context.Context, templateID string) ([5]int32, error) {
		return [5]int32{}, nil
	}
	userRepo.On("GetByID", mock.Anything, "owner-1").Return(&models.User{ID: "owner-1", DisplayName: "Owner"}, nil)

	// Owner sees the reason.
	resp, err := svc.GetTemplate(
		ContextWithUserID(context.Background(), "owner-1"),
		&pb.GetTemplateRequest{Id: "tpl-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, "policy violation", resp.Template.ModerationReason)

	// Non-owner does not.
	resp2, err := svc.GetTemplate(
		ContextWithUserID(context.Background(), "viewer-9"),
		&pb.GetTemplateRequest{Id: "tpl-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, "", resp2.Template.ModerationReason)

	// Anonymous does not.
	resp3, err := svc.GetTemplate(context.Background(), &pb.GetTemplateRequest{Id: "tpl-1"})
	require.NoError(t, err)
	assert.Equal(t, "", resp3.Template.ModerationReason)
}

func TestCheckRatingRate_WindowElapsedResets(t *testing.T) {
	svc, _, _, redis := newRatingsTestService(t)
	// Seed an expired window (expires in the past).
	_ = redis.Set(context.Background(), "rating_rate_min:u1", "20:1", 0)
	_, ok := svc.checkRatingRate(context.Background(), "u1")
	assert.True(t, ok, "expired window should reset and allow")
	// Counter reset to 1.
	v, _ := redis.Get(context.Background(), "rating_rate_min:u1")
	assert.Contains(t, v, "1:")
}

func TestCheckRatingRate_MalformedValueResets(t *testing.T) {
	svc, _, _, redis := newRatingsTestService(t)
	_ = redis.Set(context.Background(), "rating_rate_min:u1", "not-a-counter", 0)
	_, ok := svc.checkRatingRate(context.Background(), "u1")
	assert.True(t, ok, "malformed counter should reset and allow")
}

func TestCheckRatingRate_NilRedisFailOpen(t *testing.T) {
	// A service with nil Redis disables rate limiting entirely.
	svc := NewPromptService(nil, new(MockTemplateRepository), new(MockTemplateVersionRepository), nil, nil, nil, nil, nil)
	_, ok := svc.checkRatingRate(context.Background(), "u1")
	assert.True(t, ok, "nil Redis should fail open")
}
