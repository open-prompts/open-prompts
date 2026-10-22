package service

import (
	"context"
	"database/sql"
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

// MockReportRepository is a mock implementation of repository.ReportRepository.
type MockReportRepository struct {
	mock.Mock
}

func (m *MockReportRepository) Upsert(ctx context.Context, r *models.ContentReport) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}
func (m *MockReportRepository) GetMine(ctx context.Context, reporterID, itemType, itemID string) (*models.ContentReport, error) {
	args := m.Called(ctx, reporterID, itemType, itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ContentReport), args.Error(1)
}
func (m *MockReportRepository) DeleteMine(ctx context.Context, reporterID, itemType, itemID string) error {
	args := m.Called(ctx, reporterID, itemType, itemID)
	return args.Error(0)
}
func (m *MockReportRepository) ListQueue(ctx context.Context, status, itemType, reason string, limit, offset int) ([]*models.ReportGroup, int64, error) {
	args := m.Called(ctx, status, itemType, reason, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.ReportGroup), args.Get(1).(int64), args.Error(2)
}
func (m *MockReportRepository) ListForItem(ctx context.Context, itemType, itemID string) ([]*models.ContentReport, error) {
	args := m.Called(ctx, itemType, itemID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.ContentReport), args.Error(1)
}
func (m *MockReportRepository) ResolveItem(ctx context.Context, itemType, itemID, resolution, resolvedBy string) (int64, error) {
	args := m.Called(ctx, itemType, itemID, resolution, resolvedBy)
	return args.Get(0).(int64), args.Error(1)
}
func (m *MockReportRepository) CountOpen(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}
func (m *MockReportRepository) AutoResolveItem(ctx context.Context, itemType, itemID, resolvedBy string) error {
	args := m.Called(ctx, itemType, itemID, resolvedBy)
	return args.Error(0)
}

// newReportPromptService builds a PromptService wired with a report repo, a
// template repo, a comment repo, a user repo, and a fake Redis.
func newReportPromptService(t *testing.T) (*PromptService, *MockReportRepository, *MockTemplateRepository, *MockCommentRepository, *MockUserRepository, *fakeRedis) {
	t.Helper()
	reportRepo := new(MockReportRepository)
	templateRepo := new(MockTemplateRepository)
	commentRepo := new(MockCommentRepository)
	userRepo := new(MockUserRepository)
	redis := newFakeRedis()
	svc := NewPromptService(nil, templateRepo, nil, nil, nil, userRepo, commentRepo, redis)
	svc.SetReportRepo(reportRepo)
	return svc, reportRepo, templateRepo, commentRepo, userRepo, redis
}

func publicTemplate(ownerID string) *models.Template {
	return &models.Template{ID: "tpl1", OwnerID: ownerID, Visibility: "public", ModerationStatus: "normal"}
}

// expectActiveUser wires the banned-check user lookup to an active user.
func expectActiveUser(userRepo *MockUserRepository, userID string) {
	userRepo.On("GetByID", mock.Anything, userID).Return(&models.User{ID: userID, Status: "active"}, nil).Maybe()
}

func TestCreateReport(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _, _, _, _, _ := newReportPromptService(t)
		_, err := svc.CreateReport(context.Background(), &pb.CreateReportRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("BannedUser", func(t *testing.T) {
		svc, _, _, _, userRepo, _ := newReportPromptService(t)
		userRepo.On("GetByID", mock.Anything, "u1").Return(&models.User{ID: "u1", Status: "banned"}, nil).Once()
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "spam"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("InvalidItemType", func(t *testing.T) {
		svc, _, _, _, _, _ := newReportPromptService(t)
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "bogus", ItemId: "tpl1", Reason: "spam"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("InvalidReason", func(t *testing.T) {
		svc, _, _, _, _, _ := newReportPromptService(t)
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "bogus"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("NoteTooLong", func(t *testing.T) {
		svc, _, _, _, _, _ := newReportPromptService(t)
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "spam", Note: string(make([]rune, 501))})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("TemplateNotFound", func(t *testing.T) {
		svc, _, templateRepo, _, userRepo, _ := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return nil, repository.ErrTemplateNotFound
		}
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "spam"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("OwnTemplate", func(t *testing.T) {
		svc, _, templateRepo, _, userRepo, _ := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return publicTemplate("u1"), nil
		}
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "spam"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("CommentNotFound", func(t *testing.T) {
		svc, _, _, commentRepo, userRepo, _ := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		commentRepo.On("Get", mock.Anything, "c1").Return(nil, sql.ErrNoRows).Once()
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "comment", ItemId: "c1", Reason: "spam"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("OwnComment", func(t *testing.T) {
		svc, _, templateRepo, commentRepo, userRepo, _ := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		commentRepo.On("Get", mock.Anything, "c1").Return(&models.Comment{ID: "c1", UserID: "u1", TemplateID: "tpl1"}, nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return publicTemplate("u2"), nil
		}
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "comment", ItemId: "c1", Reason: "spam"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("RateLimitExceeded", func(t *testing.T) {
		svc, _, templateRepo, _, userRepo, redis := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return publicTemplate("u2"), nil
		}
		// Pre-seed the counter at the limit.
		redis.data["report_rate:u1"] = "20:9999999999"
		_, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "spam"})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})

	t.Run("SuccessTemplate", func(t *testing.T) {
		svc, reportRepo, templateRepo, _, userRepo, _ := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return publicTemplate("u2"), nil
		}
		reportRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(r *models.ContentReport) bool {
			return r.ItemType == "template" && r.ItemID == "tpl1" && r.ReporterID == "u1" && r.Reason == "spam" && r.Status == "open"
		})).Return(nil).Once()
		resp, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "spam", Note: "  note  "})
		assert.NoError(t, err)
		assert.Equal(t, "template", resp.ItemType)
		assert.Equal(t, "note", resp.Note)
	})

	t.Run("SuccessComment", func(t *testing.T) {
		svc, reportRepo, templateRepo, commentRepo, userRepo, _ := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		commentRepo.On("Get", mock.Anything, "c1").Return(&models.Comment{ID: "c1", UserID: "u2", TemplateID: "tpl1"}, nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return publicTemplate("u3"), nil
		}
		reportRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(r *models.ContentReport) bool {
			return r.ItemType == "comment" && r.ItemID == "c1" && r.ReporterID == "u1"
		})).Return(nil).Once()
		resp, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "comment", ItemId: "c1", Reason: "offensive"})
		assert.NoError(t, err)
		assert.Equal(t, "comment", resp.ItemType)
	})
}

func TestGetMyReport(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _, _, _, _, _ := newReportPromptService(t)
		_, err := svc.GetMyReport(context.Background(), &pb.GetMyReportRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NoReport", func(t *testing.T) {
		svc, reportRepo, _, _, _, _ := newReportPromptService(t)
		reportRepo.On("GetMine", mock.Anything, "u1", "template", "tpl1").Return(nil, repository.ErrReportNotFound).Once()
		resp, err := svc.GetMyReport(ContextWithUserID(context.Background(), "u1"),
			&pb.GetMyReportRequest{ItemType: "template", ItemId: "tpl1"})
		assert.NoError(t, err)
		assert.Nil(t, resp.Report)
	})

	t.Run("HasReport", func(t *testing.T) {
		svc, reportRepo, _, _, _, _ := newReportPromptService(t)
		reportRepo.On("GetMine", mock.Anything, "u1", "template", "tpl1").Return(&models.ContentReport{
			ID: "r1", ItemType: "template", ItemID: "tpl1", ReporterID: "u1", Reason: "spam", Status: "open", CreatedAt: time.Now(),
		}, nil).Once()
		resp, err := svc.GetMyReport(ContextWithUserID(context.Background(), "u1"),
			&pb.GetMyReportRequest{ItemType: "template", ItemId: "tpl1"})
		assert.NoError(t, err)
		assert.NotNil(t, resp.Report)
		assert.Equal(t, "r1", resp.Report.Id)
	})
}

func TestWithdrawReport(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _, _, _, _, _ := newReportPromptService(t)
		_, err := svc.WithdrawReport(context.Background(), &pb.WithdrawReportRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, reportRepo, _, _, _, _ := newReportPromptService(t)
		reportRepo.On("DeleteMine", mock.Anything, "u1", "template", "tpl1").Return(nil).Once()
		resp, err := svc.WithdrawReport(ContextWithUserID(context.Background(), "u1"),
			&pb.WithdrawReportRequest{ItemType: "template", ItemId: "tpl1"})
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	})
}

func TestDeleteCommentAutoResolvesReports(t *testing.T) {
	// FR-12: deleting a comment auto-resolves its open reports as actioned.
	svc, reportRepo, templateRepo, commentRepo, _, _ := newReportPromptService(t)
	commentRepo.On("Get", mock.Anything, "c1").Return(&models.Comment{ID: "c1", UserID: "u1", TemplateID: "tpl1"}, nil).Once()
	templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
		return publicTemplate("u1"), nil
	}
	commentRepo.On("Delete", mock.Anything, "c1").Return(true, int64(0), nil).Once()
	reportRepo.On("AutoResolveItem", mock.Anything, "comment", "c1", "u1").Return(nil).Once()
	resp, err := svc.DeleteComment(ContextWithUserID(context.Background(), "u1"),
		&pb.DeleteCommentRequest{CommentId: "c1"})
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	reportRepo.AssertExpectations(t)
}

func TestReportRateLimitConfig(t *testing.T) {
	t.Setenv("REPORT_RATE_PER_HOUR", "5")
	assert.Equal(t, 5, reportRatePerHour())
	t.Setenv("REPORT_RATE_PER_HOUR", "0")
	assert.Equal(t, reportRatePerHourDef, reportRatePerHour())
	t.Setenv("REPORT_RATE_PER_HOUR", "")
	assert.Equal(t, reportRatePerHourDef, reportRatePerHour())
}

func TestCheckReportRate(t *testing.T) {
	t.Run("FailOpenWhenNil", func(t *testing.T) {
		svc := &PromptService{Redis: nil}
		_, ok := svc.checkReportRate(context.Background(), "u1")
		assert.True(t, ok, "nil Redis must fail open")
	})

	t.Run("WindowElapsedResets", func(t *testing.T) {
		svc, _, templateRepo, _, userRepo, redis := newReportPromptService(t)
		expectActiveUser(userRepo, "u1")
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return publicTemplate("u2"), nil
		}
		// Pre-seed an expired window.
		redis.data["report_rate:u1"] = "20:1"
		reportRepo := svc.ReportRepo.(*MockReportRepository)
		reportRepo.On("Upsert", mock.Anything, mock.Anything).Return(nil).Once()
		resp, err := svc.CreateReport(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateReportRequest{ItemType: "template", ItemId: "tpl1", Reason: "spam"})
		assert.NoError(t, err)
		assert.Equal(t, "template", resp.ItemType)
	})
}