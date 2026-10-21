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

// MockAppealRepository is a mock implementation of repository.AppealRepository.
type MockAppealRepository struct {
	mock.Mock
}

func (m *MockAppealRepository) CreateAppeal(ctx context.Context, a *models.Appeal) error {
	args := m.Called(ctx, a)
	return args.Error(0)
}
func (m *MockAppealRepository) GetAppeal(ctx context.Context, appealID string) (*models.Appeal, error) {
	args := m.Called(ctx, appealID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Appeal), args.Error(1)
}
func (m *MockAppealRepository) GetAppealByAction(ctx context.Context, actionID string) (*models.Appeal, error) {
	args := m.Called(ctx, actionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Appeal), args.Error(1)
}
func (m *MockAppealRepository) ListAppeals(ctx context.Context, appellantID string, page, perPage int) ([]models.Appeal, int64, error) {
	args := m.Called(ctx, appellantID, page, perPage)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]models.Appeal), args.Get(1).(int64), args.Error(2)
}
func (m *MockAppealRepository) GetModerationAction(ctx context.Context, actionID string) (*models.ModerationAction, error) {
	args := m.Called(ctx, actionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ModerationAction), args.Error(1)
}
func (m *MockAppealRepository) ListModerationActionsForTemplate(ctx context.Context, templateID string) ([]models.ModerationAction, error) {
	args := m.Called(ctx, templateID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.ModerationAction), args.Error(1)
}
func (m *MockAppealRepository) ListQueue(ctx context.Context, state string, page, perPage int) ([]models.Appeal, int64, error) {
	args := m.Called(ctx, state, page, perPage)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]models.Appeal), args.Get(1).(int64), args.Error(2)
}
func (m *MockAppealRepository) CountByState(ctx context.Context) (int, int, int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Int(1), args.Int(2), args.Error(3)
}
func (m *MockAppealRepository) SetState(ctx context.Context, appealID, state string) error {
	args := m.Called(ctx, appealID, state)
	return args.Error(0)
}
func (m *MockAppealRepository) ResolveAppeal(ctx context.Context, appealID, outcome, note, resolverID string) error {
	args := m.Called(ctx, appealID, outcome, note, resolverID)
	return args.Error(0)
}
func (m *MockAppealRepository) MarkActionSuperseded(ctx context.Context, actionID string) error {
	args := m.Called(ctx, actionID)
	return args.Error(0)
}

func newAppealPromptService(t *testing.T) (*PromptService, *MockAppealRepository, *MockTemplateRepository, *MockRedisStore) {
	t.Helper()
	appealRepo := new(MockAppealRepository)
	templateRepo := new(MockTemplateRepository)
	redis := new(MockRedisStore)
	svc := NewPromptService(nil, templateRepo, nil, nil, nil, nil, nil, redis)
	svc.SetAppealRepo(appealRepo)
	return svc, appealRepo, templateRepo, redis
}

func makeAction(id, templateID, actorID, status string) *models.ModerationAction {
	return &models.ModerationAction{
		ID:         id,
		TemplateID: templateID,
		ActionType: "soft_delete",
		ActorID:    actorID,
		ActorEmail: actorID + "@example.com",
		Reason:     "spam",
		Status:     status,
		CreatedAt:  time.Now().Add(-time.Hour),
	}
}

func TestCreateAppeal(t *testing.T) {
	statement := "This decision is wrong because the links point to my company's product documentation, not off-platform promotion, and the template follows the community guidelines."
	action := makeAction("act1", "tpl1", "admin1", "active")

	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _, _, _ := newAppealPromptService(t)
		_, err := svc.CreateAppeal(context.Background(), &pb.CreateAppealRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("StatementTooShort", func(t *testing.T) {
		svc, _, _, _ := newAppealPromptService(t)
		_, err := svc.CreateAppeal(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateAppealRequest{ModerationActionId: "act1", Statement: "short"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("ActionNotFound", func(t *testing.T) {
		svc, appealRepo, _, _ := newAppealPromptService(t)
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(nil, repository.ErrActionNotFound).Once()
		_, err := svc.CreateAppeal(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateAppealRequest{ModerationActionId: "act1", Statement: statement})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("NotOwnersTemplate", func(t *testing.T) {
		svc, appealRepo, templateRepo, _ := newAppealPromptService(t)
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return &models.Template{ID: "tpl1", OwnerID: "someone_else"}, nil
		}
		_, err := svc.CreateAppeal(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateAppealRequest{ModerationActionId: "act1", Statement: statement})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("SupersededAction", func(t *testing.T) {
		svc, appealRepo, templateRepo, _ := newAppealPromptService(t)
		sup := makeAction("act1", "tpl1", "admin1", "superseded")
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(sup, nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return &models.Template{ID: "tpl1", OwnerID: "u1"}, nil
		}
		_, err := svc.CreateAppeal(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateAppealRequest{ModerationActionId: "act1", Statement: statement})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("RateLimitExceeded", func(t *testing.T) {
		svc, appealRepo, templateRepo, redis := newAppealPromptService(t)
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return &models.Template{ID: "tpl1", OwnerID: "u1"}, nil
		}
		appealRepo.On("GetAppealByAction", mock.Anything, "act1").Return(nil, repository.ErrAppealNotFound).Once()
		redis.On("Get", mock.Anything, "appeal_open:u1").Return("5:9999999999", nil).Once()
		_, err := svc.CreateAppeal(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateAppealRequest{ModerationActionId: "act1", Statement: statement})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})

	t.Run("AlreadyAppealed", func(t *testing.T) {
		svc, appealRepo, templateRepo, _ := newAppealPromptService(t)
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return &models.Template{ID: "tpl1", OwnerID: "u1"}, nil
		}
		appealRepo.On("GetAppealByAction", mock.Anything, "act1").Return(&models.Appeal{ID: "existing"}, nil).Once()
		_, err := svc.CreateAppeal(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateAppealRequest{ModerationActionId: "act1", Statement: statement})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, appealRepo, templateRepo, redis := newAppealPromptService(t)
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return &models.Template{ID: "tpl1", OwnerID: "u1"}, nil
		}
		appealRepo.On("GetAppealByAction", mock.Anything, "act1").Return(nil, repository.ErrAppealNotFound).Once()
		redis.On("Get", mock.Anything, "appeal_open:u1").Return("", nil).Once()
		redis.On("Set", mock.Anything, "appeal_open:u1", mock.Anything, mock.Anything).Return(nil).Once()
		appealRepo.On("CreateAppeal", mock.Anything, mock.MatchedBy(func(a *models.Appeal) bool {
			return a.AppellantID == "u1" && a.ModerationActionID == "act1" && a.State == "open"
		})).Return(nil).Once()
		appealRepo.On("GetAppeal", mock.Anything, "").Return(&models.Appeal{
			ID: "appeal1", ModerationActionID: "act1", AppellantID: "u1",
			Statement: statement, State: "open", CreatedAt: time.Now(),
			TemplateID: "tpl1", ActionType: "soft_delete", ActionReason: "spam",
			ActorEmail: "admin1@example.com", ActionCreatedAt: action.CreatedAt, ActionStatus: "active",
		}, nil).Once()

		resp, err := svc.CreateAppeal(ContextWithUserID(context.Background(), "u1"),
			&pb.CreateAppealRequest{ModerationActionId: "act1", Statement: statement})
		assert.NoError(t, err)
		assert.Equal(t, "appeal1", resp.Id)
		assert.Equal(t, "open", resp.State)
	})
}

func TestListAppeals(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _, _, _ := newAppealPromptService(t)
		_, err := svc.ListAppeals(context.Background(), &pb.ListAppealsRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, appealRepo, _, _ := newAppealPromptService(t)
		appealRepo.On("ListAppeals", mock.Anything, "u1", 1, 20).Return([]models.Appeal{
			{ID: "a1", State: "open", TemplateID: "t1", ActionType: "lock"},
		}, int64(1), nil).Once()
		resp, err := svc.ListAppeals(ContextWithUserID(context.Background(), "u1"), &pb.ListAppealsRequest{})
		assert.NoError(t, err)
		assert.Len(t, resp.Appeals, 1)
		assert.Equal(t, int64(1), resp.Total)
	})
}

func TestGetAppeal(t *testing.T) {
	t.Run("RequiresAuth", func(t *testing.T) {
		svc, _, _, _ := newAppealPromptService(t)
		_, err := svc.GetAppeal(context.Background(), &pb.GetAppealRequest{AppealId: "a1"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, appealRepo, _, _ := newAppealPromptService(t)
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(nil, repository.ErrAppealNotFound).Once()
		_, err := svc.GetAppeal(ContextWithUserID(context.Background(), "u1"), &pb.GetAppealRequest{AppealId: "a1"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("NotOwner", func(t *testing.T) {
		svc, appealRepo, _, _ := newAppealPromptService(t)
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(&models.Appeal{
			ID: "a1", AppellantID: "other", ModerationActionID: "act1",
		}, nil).Once()
		_, err := svc.GetAppeal(ContextWithUserID(context.Background(), "u1"), &pb.GetAppealRequest{AppealId: "a1"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, appealRepo, templateRepo, _ := newAppealPromptService(t)
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(&models.Appeal{
			ID: "a1", AppellantID: "u1", ModerationActionID: "act1", Statement: "stmt", State: "open",
		}, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(actionForGet(), nil).Once()
		templateRepo.GetFunc = func(ctx context.Context, id, uid string) (*models.Template, error) {
			return &models.Template{ID: "tpl1", Title: "My Template"}, nil
		}
		resp, err := svc.GetAppeal(ContextWithUserID(context.Background(), "u1"), &pb.GetAppealRequest{AppealId: "a1"})
		assert.NoError(t, err)
		assert.Equal(t, "a1", resp.Id)
		assert.Equal(t, "My Template", resp.TemplateTitle)
	})
}

func actionForGet() *models.ModerationAction {
	return &models.ModerationAction{
		ID: "act1", TemplateID: "tpl1", ActionType: "unpublish", ActorID: "admin1",
		ActorEmail: "admin1@example.com", Reason: "spam", Status: "active", CreatedAt: time.Now(),
	}
}

func TestWriteAppealAudit(t *testing.T) {
	t.Run("NilRepoNoop", func(t *testing.T) {
		svc := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
		svc.writeAppealAudit(context.Background(), "u1", makeAction("act1", "tpl1", "admin1", "active"), "appeal1")
	})

	t.Run("WritesAudit", func(t *testing.T) {
		svc, _, _, _ := newAppealPromptService(t)
		mockAdmin := new(MockAdminRepository)
		svc.SetAdminRepo(mockAdmin)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.MatchedBy(func(e *models.AdminAuditLog) bool {
			return e.Action == "appeal.create" && e.TargetType == "template" && e.TargetID == "tpl1" && e.Reason == "appeal=appeal1"
		})).Return(nil).Once()
		svc.writeAppealAudit(context.Background(), "u1", makeAction("act1", "tpl1", "admin1", "active"), "appeal1")
		mockAdmin.AssertExpectations(t)
	})
}

func TestAppealMaxOpenDefault(t *testing.T) {
	t.Setenv("APPEAL_MAX_OPEN", "")
	assert.Equal(t, 5, appealMaxOpen())
	t.Setenv("APPEAL_MAX_OPEN", "3")
	assert.Equal(t, 3, appealMaxOpen())
	t.Setenv("APPEAL_MAX_OPEN", "0")
	assert.Equal(t, 5, appealMaxOpen())
}

func TestCheckOpenAppealLimit(t *testing.T) {
	t.Run("NilRedisFailOpen", func(t *testing.T) {
		svc := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
		_, ok := svc.checkOpenAppealLimit(context.Background(), "u1")
		assert.True(t, ok)
	})

	t.Run("OverLimit", func(t *testing.T) {
		svc, _, _, redis := newAppealPromptService(t)
		redis.On("Get", mock.Anything, "appeal_open:u1").Return("5:9999999999", nil).Once()
		_, ok := svc.checkOpenAppealLimit(context.Background(), "u1")
		assert.False(t, ok)
	})

	t.Run("UnderLimit", func(t *testing.T) {
		svc, _, _, redis := newAppealPromptService(t)
		redis.On("Get", mock.Anything, "appeal_open:u1").Return("2:9999999999", nil).Once()
		redis.On("Set", mock.Anything, "appeal_open:u1", mock.Anything, mock.Anything).Return(nil).Once()
		_, ok := svc.checkOpenAppealLimit(context.Background(), "u1")
		assert.True(t, ok)
	})
}

func TestDecrementOpenAppeal(t *testing.T) {
	t.Run("NilRedisNoop", func(t *testing.T) {
		svc := NewPromptService(nil, nil, nil, nil, nil, nil, nil, nil)
		svc.decrementOpenAppeal(context.Background(), "u1") // should not panic
	})

	t.Run("Decrements", func(t *testing.T) {
		svc, _, _, redis := newAppealPromptService(t)
		redis.On("Get", mock.Anything, "appeal_open:u1").Return("3:9999999999", nil).Once()
		redis.On("Set", mock.Anything, "appeal_open:u1", mock.Anything, mock.Anything).Return(nil).Once()
		svc.decrementOpenAppeal(context.Background(), "u1")
	})

	t.Run("DeletesAtZero", func(t *testing.T) {
		svc, _, _, redis := newAppealPromptService(t)
		redis.On("Get", mock.Anything, "appeal_open:u1").Return("1:9999999999", nil).Once()
		redis.On("Del", mock.Anything, "appeal_open:u1").Return(nil).Once()
		svc.decrementOpenAppeal(context.Background(), "u1")
	})
}