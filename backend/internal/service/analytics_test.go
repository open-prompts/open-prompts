package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// metadataCtxWithIP returns a context carrying the x-client-ip metadata.
func metadataCtxWithIP(ip string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-client-ip", ip))
}

// MockAnalyticsRepo is a mock implementation of AnalyticsRepo.
type MockAnalyticsRepo struct {
	mock.Mock
}

func (m *MockAnalyticsRepo) InsertEvent(ctx context.Context, templateID, eventType, source, userID, sessionID string) error {
	args := m.Called(ctx, templateID, eventType, source, userID, sessionID)
	return args.Error(0)
}
func (m *MockAnalyticsRepo) RecentViewExists(ctx context.Context, templateID, sessionID string, window time.Duration) (bool, error) {
	args := m.Called(ctx, templateID, sessionID, window)
	return args.Bool(0), args.Error(1)
}
func (m *MockAnalyticsRepo) IncrementCounter(ctx context.Context, templateID, column string) error {
	args := m.Called(ctx, templateID, column)
	return args.Error(0)
}
func (m *MockAnalyticsRepo) DailyEventSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string][2]int32, error) {
	args := m.Called(ctx, templateID, ownerID, from, to)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][2]int32), args.Error(1)
}
func (m *MockAnalyticsRepo) DailyLikeSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error) {
	args := m.Called(ctx, templateID, ownerID, from, to)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int32), args.Error(1)
}
func (m *MockAnalyticsRepo) DailyFavoriteSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error) {
	args := m.Called(ctx, templateID, ownerID, from, to)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int32), args.Error(1)
}
func (m *MockAnalyticsRepo) UseSourceSplit(ctx context.Context, templateID, ownerID string, from, to time.Time) (int32, int32, error) {
	args := m.Called(ctx, templateID, ownerID, from, to)
	return args.Get(0).(int32), args.Get(1).(int32), args.Error(2)
}
func (m *MockAnalyticsRepo) TopTemplatesByUses(ctx context.Context, ownerID string, from, to time.Time, limit int) ([]models.TopTemplateRow, error) {
	args := m.Called(ctx, ownerID, from, to, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TopTemplateRow), args.Error(1)
}
func (m *MockAnalyticsRepo) CountOwnerTemplates(ctx context.Context, ownerID string) (int32, error) {
	args := m.Called(ctx, ownerID)
	return args.Get(0).(int32), args.Error(1)
}

func newAnalyticsTestService(t *testing.T) (*PromptService, *MockTemplateRepository, *MockAnalyticsRepo, *fakeRedis) {
	t.Helper()
	templateRepo := new(MockTemplateRepository)
	analyticsRepo := new(MockAnalyticsRepo)
	redis := newFakeRedis()
	svc := NewPromptService(nil, templateRepo, nil, nil, nil, nil, nil, redis)
	svc.SetAnalyticsRepo(analyticsRepo)
	return svc, templateRepo, analyticsRepo, redis
}

func analyticsTestTemplate(ownerID string) *models.Template {
	return &models.Template{
		ID:         "tpl-1",
		OwnerID:    ownerID,
		Visibility: "public",
	}
}

// ---- RecordTemplateEvent ----

func TestRecordTemplateEvent_MissingTemplateID(t *testing.T) {
	svc, _, _, _ := newAnalyticsTestService(t)
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{EventType: "view"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRecordTemplateEvent_UnsupportedEventType(t *testing.T) {
	svc, _, _, _ := newAnalyticsTestService(t)
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "click"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRecordTemplateEvent_TemplateNotFound(t *testing.T) {
	svc, templateRepo, _, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return nil, errors.New("template not found")
	}
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestRecordTemplateEvent_PrivateNonOwner(t *testing.T) {
	svc, templateRepo, _, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "private"}, nil
	}
	ctx := ContextWithUserID(context.Background(), "other-user")
	_, err := svc.RecordTemplateEvent(ctx, &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestRecordTemplateEvent_OwnerPrivateAllowed(t *testing.T) {
	svc, templateRepo, analyticsRepo, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "private"}, nil
	}
	analyticsRepo.On("InsertEvent", mock.Anything, "tpl-1", "view", "web", "owner-1", "s1").Return(nil)
	analyticsRepo.On("IncrementCounter", mock.Anything, "tpl-1", "view_count").Return(nil)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	_, err := svc.RecordTemplateEvent(ctx, &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view", SessionId: "s1"})
	assert.NoError(t, err)
	analyticsRepo.AssertExpectations(t)
}

func TestRecordTemplateEvent_DedupRedisHit(t *testing.T) {
	svc, templateRepo, analyticsRepo, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return analyticsTestTemplate("owner-1"), nil
	}
	// First call sets the key (SetNX true → not duplicate).
	analyticsRepo.On("InsertEvent", mock.Anything, "tpl-1", "view", "web", "", "s1").Return(nil).Once()
	analyticsRepo.On("IncrementCounter", mock.Anything, "tpl-1", "view_count").Return(nil).Once()
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view", SessionId: "s1"})
	assert.NoError(t, err)
	// Second call: key exists → SetNX false → duplicate, no insert.
	_, err = svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view", SessionId: "s1"})
	assert.NoError(t, err)
	analyticsRepo.AssertNumberOfCalls(t, "InsertEvent", 1)
}

func TestRecordTemplateEvent_DedupRedisDownDBFallback(t *testing.T) {
	svc, templateRepo, analyticsRepo, redis := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return analyticsTestTemplate("owner-1"), nil
	}
	redis.fail = true
	// Redis down → DB fallback: RecentViewExists true → duplicate.
	analyticsRepo.On("RecentViewExists", mock.Anything, "tpl-1", "s1", viewDedupWindow).Return(true, nil)
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view", SessionId: "s1"})
	assert.NoError(t, err)
	analyticsRepo.AssertNumberOfCalls(t, "InsertEvent", 0)
}

func TestRecordTemplateEvent_DedupRedisDownDBMiss(t *testing.T) {
	svc, templateRepo, analyticsRepo, redis := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return analyticsTestTemplate("owner-1"), nil
	}
	redis.fail = true
	analyticsRepo.On("RecentViewExists", mock.Anything, "tpl-1", "s1", viewDedupWindow).Return(false, nil)
	analyticsRepo.On("InsertEvent", mock.Anything, "tpl-1", "view", "web", "", "s1").Return(nil)
	analyticsRepo.On("IncrementCounter", mock.Anything, "tpl-1", "view_count").Return(nil)
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view", SessionId: "s1"})
	assert.NoError(t, err)
	analyticsRepo.AssertExpectations(t)
}

func TestRecordTemplateEvent_InsertError(t *testing.T) {
	svc, templateRepo, analyticsRepo, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return analyticsTestTemplate("owner-1"), nil
	}
	analyticsRepo.On("InsertEvent", mock.Anything, "tpl-1", "view", "web", "", "").Return(errors.New("db down"))
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestRecordTemplateEvent_RateLimited(t *testing.T) {
	svc, templateRepo, _, redis := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return analyticsTestTemplate("owner-1"), nil
	}
	// Pre-seed the rate counter at the max.
	redis.data["tpl_event_rate:unknown"] = "60:" + strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	_, err := svc.RecordTemplateEvent(context.Background(), &pb.RecordTemplateEventRequest{TemplateId: "tpl-1", EventType: "view"})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
}

// ---- RecordUseEvent ----

func TestRecordUseEvent_EmptyTemplate(t *testing.T) {
	svc, _, analyticsRepo, _ := newAnalyticsTestService(t)
	svc.RecordUseEvent(context.Background(), "", "u1", "web")
	analyticsRepo.AssertNotCalled(t, "InsertEvent")
}

func TestRecordUseEvent_InvalidSource(t *testing.T) {
	svc, _, analyticsRepo, _ := newAnalyticsTestService(t)
	svc.RecordUseEvent(context.Background(), "tpl-1", "u1", "mobile")
	analyticsRepo.AssertNotCalled(t, "InsertEvent")
}

func TestRecordUseEvent_Success(t *testing.T) {
	svc, _, analyticsRepo, _ := newAnalyticsTestService(t)
	analyticsRepo.On("InsertEvent", mock.Anything, "tpl-1", "use", "web", "u1", "").Return(nil)
	analyticsRepo.On("IncrementCounter", mock.Anything, "tpl-1", "use_count").Return(nil)
	svc.RecordUseEvent(context.Background(), "tpl-1", "u1", "web")
	analyticsRepo.AssertExpectations(t)
}

func TestRecordUseEvent_InsertErrorLogged(t *testing.T) {
	svc, _, analyticsRepo, _ := newAnalyticsTestService(t)
	analyticsRepo.On("InsertEvent", mock.Anything, "tpl-1", "use", "api", "u1", "").Return(errors.New("db down"))
	// Must not panic; error is logged only.
	svc.RecordUseEvent(context.Background(), "tpl-1", "u1", "api")
	analyticsRepo.AssertExpectations(t)
}

// ---- GetTemplateStats ----

func TestGetTemplateStats_Unauthenticated(t *testing.T) {
	svc, _, _, _ := newAnalyticsTestService(t)
	_, err := svc.GetTemplateStats(context.Background(), &pb.GetTemplateStatsRequest{TemplateId: "tpl-1", Days: 30})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGetTemplateStats_InvalidDays(t *testing.T) {
	svc, _, _, _ := newAnalyticsTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	_, err := svc.GetTemplateStats(ctx, &pb.GetTemplateStatsRequest{TemplateId: "tpl-1", Days: 14})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetTemplateStats_NotFound(t *testing.T) {
	svc, templateRepo, _, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return nil, errors.New("template not found")
	}
	ctx := ContextWithUserID(context.Background(), "owner-1")
	_, err := svc.GetTemplateStats(ctx, &pb.GetTemplateStatsRequest{TemplateId: "tpl-1", Days: 30})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetTemplateStats_PrivateNonOwner(t *testing.T) {
	svc, templateRepo, _, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return &models.Template{ID: "tpl-1", OwnerID: "owner-1", Visibility: "private"}, nil
	}
	ctx := ContextWithUserID(context.Background(), "other")
	_, err := svc.GetTemplateStats(ctx, &pb.GetTemplateStatsRequest{TemplateId: "tpl-1", Days: 30})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetTemplateStats_PublicNonOwner(t *testing.T) {
	svc, templateRepo, _, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return analyticsTestTemplate("owner-1"), nil
	}
	ctx := ContextWithUserID(context.Background(), "other")
	_, err := svc.GetTemplateStats(ctx, &pb.GetTemplateStatsRequest{TemplateId: "tpl-1", Days: 30})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestGetTemplateStats_Success(t *testing.T) {
	svc, templateRepo, analyticsRepo, _ := newAnalyticsTestService(t)
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return analyticsTestTemplate("owner-1"), nil
	}
	analyticsRepo.On("DailyEventSeries", mock.Anything, "tpl-1", "", mock.Anything, mock.Anything).Return(map[string][2]int32{}, nil)
	analyticsRepo.On("DailyLikeSeries", mock.Anything, "tpl-1", "", mock.Anything, mock.Anything).Return(map[string]int32{}, nil)
	analyticsRepo.On("DailyFavoriteSeries", mock.Anything, "tpl-1", "", mock.Anything, mock.Anything).Return(map[string]int32{}, nil)
	analyticsRepo.On("UseSourceSplit", mock.Anything, "tpl-1", "", mock.Anything, mock.Anything).Return(int32(5), int32(2), nil)

	ctx := ContextWithUserID(context.Background(), "owner-1")
	resp, err := svc.GetTemplateStats(ctx, &pb.GetTemplateStatsRequest{TemplateId: "tpl-1", Days: 7})
	assert.NoError(t, err)
	assert.Len(t, resp.Series, 7)
	assert.Equal(t, int32(5), resp.SourceSplit.Web)
	assert.Equal(t, int32(2), resp.SourceSplit.Api)
	assert.Equal(t, 0.0, resp.ViewToUseRate)
}

// ---- GetCreatorStats ----

func TestGetCreatorStats_Unauthenticated(t *testing.T) {
	svc, _, _, _ := newAnalyticsTestService(t)
	_, err := svc.GetCreatorStats(context.Background(), &pb.GetCreatorStatsRequest{Days: 30})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGetCreatorStats_InvalidDays(t *testing.T) {
	svc, _, _, _ := newAnalyticsTestService(t)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	_, err := svc.GetCreatorStats(ctx, &pb.GetCreatorStatsRequest{Days: 5})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetCreatorStats_Success(t *testing.T) {
	svc, _, analyticsRepo, _ := newAnalyticsTestService(t)
	analyticsRepo.On("DailyEventSeries", mock.Anything, "", "owner-1", mock.Anything, mock.Anything).Return(map[string][2]int32{}, nil)
	analyticsRepo.On("DailyLikeSeries", mock.Anything, "", "owner-1", mock.Anything, mock.Anything).Return(map[string]int32{}, nil)
	analyticsRepo.On("DailyFavoriteSeries", mock.Anything, "", "owner-1", mock.Anything, mock.Anything).Return(map[string]int32{}, nil)
	analyticsRepo.On("UseSourceSplit", mock.Anything, "", "owner-1", mock.Anything, mock.Anything).Return(int32(10), int32(3), nil)
	analyticsRepo.On("TopTemplatesByUses", mock.Anything, "owner-1", mock.Anything, mock.Anything, 10).Return([]models.TopTemplateRow{
		{ID: "t1", Title: "T1", Views: 10, Uses: 5, Likes: 2, Favorites: 1},
	}, nil)
	analyticsRepo.On("CountOwnerTemplates", mock.Anything, "owner-1").Return(int32(3), nil)

	ctx := ContextWithUserID(context.Background(), "owner-1")
	resp, err := svc.GetCreatorStats(ctx, &pb.GetCreatorStatsRequest{Days: 30})
	assert.NoError(t, err)
	assert.Len(t, resp.Series, 30)
	assert.Equal(t, int32(3), resp.TemplateCount)
	assert.Len(t, resp.TopTemplates, 1)
	assert.Equal(t, 0.5, resp.TopTemplates[0].Rate)
}

func TestGetCreatorStats_RepoError(t *testing.T) {
	svc, _, analyticsRepo, _ := newAnalyticsTestService(t)
	analyticsRepo.On("DailyEventSeries", mock.Anything, "", "owner-1", mock.Anything, mock.Anything).Return(nil, errors.New("boom"))
	ctx := ContextWithUserID(context.Background(), "owner-1")
	_, err := svc.GetCreatorStats(ctx, &pb.GetCreatorStatsRequest{Days: 30})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestGetCreatorStats_NoRepo(t *testing.T) {
	svc := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	ctx := ContextWithUserID(context.Background(), "owner-1")
	_, err := svc.GetCreatorStats(ctx, &pb.GetCreatorStatsRequest{Days: 30})
	assert.Equal(t, codes.Internal, status.Code(err))
}

// ---- zeroFill / sumSeries ----

func TestZeroFill_ExactDays(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ev := map[string][2]int32{"2025-01-01": {5, 2}}
	lk := map[string]int32{"2025-01-01": 1}
	fv := map[string]int32{}
	points := zeroFill(3, from, ev, lk, fv)
	assert.Len(t, points, 3)
	assert.Equal(t, "2025-01-01", points[0].Date)
	assert.Equal(t, "2025-01-03", points[2].Date)
	assert.Equal(t, int32(5), points[0].Views)
	assert.Equal(t, int32(2), points[0].Uses)
	assert.Equal(t, int32(1), points[0].Likes)
	assert.Equal(t, int32(0), points[2].Views)
}

func TestSumSeries(t *testing.T) {
	series := []*pb.DailyMetricPoint{
		{Date: "a", Views: 1, Uses: 2, Likes: 3, Favorites: 4},
		{Date: "b", Views: 5, Uses: 6, Likes: 7, Favorites: 8},
	}
	sum := sumSeries(series)
	assert.Equal(t, int32(6), sum.Views)
	assert.Equal(t, int32(8), sum.Uses)
	assert.Equal(t, int32(10), sum.Likes)
	assert.Equal(t, int32(12), sum.Favorites)
}

// ---- checkEventRate ----

func TestCheckEventRate_FailOpenNoRedis(t *testing.T) {
	svc := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
	retry, ok := svc.checkEventRate(context.Background())
	assert.True(t, ok)
	assert.Equal(t, 0, retry)
}

func TestCheckEventRate_UnderLimit(t *testing.T) {
	svc, _, _, _ := newAnalyticsTestService(t)
	retry, ok := svc.checkEventRate(context.Background())
	assert.True(t, ok)
	assert.Equal(t, 0, retry)
}

func TestCheckEventRate_OverLimit(t *testing.T) {
	svc, _, _, redis := newAnalyticsTestService(t)
	redis.data["tpl_event_rate:1.2.3.4"] = "60:" + strconv.FormatInt(time.Now().Add(30*time.Second).Unix(), 10)
	ctx := metadataCtxWithIP("1.2.3.4")
	retry, ok := svc.checkEventRate(ctx)
	assert.False(t, ok)
	assert.True(t, retry > 0)
}

func TestCheckEventRate_RedisDownFailOpen(t *testing.T) {
	svc, _, _, redis := newAnalyticsTestService(t)
	redis.fail = true
	ctx := metadataCtxWithIP("1.2.3.4")
	retry, ok := svc.checkEventRate(ctx)
	assert.True(t, ok)
	assert.Equal(t, 0, retry)
}