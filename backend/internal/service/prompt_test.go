package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mocks
type MockPromptRepository struct {
	mock.Mock
}

func (m *MockPromptRepository) Create(ctx context.Context, prompt *models.Prompt) error {
	args := m.Called(ctx, prompt)
	return args.Error(0)
}

func (m *MockPromptRepository) Get(ctx context.Context, id string) (*models.Prompt, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Prompt), args.Error(1)
}

func (m *MockPromptRepository) List(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*models.Prompt, error) {
	args := m.Called(ctx, limit, offset, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Prompt), args.Error(1)
}

func (m *MockPromptRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// MockTemplateRepository (minimal for Prompt tests)
type MockTemplateRepository struct {
	mock.Mock
	CreateFunc             func(ctx context.Context, t *models.Template) error
	GetFunc                func(ctx context.Context, id string, currentUserID string) (*models.Template, error)
	ListFunc               func(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*models.Template, error)
	ToggleLikeFunc         func(ctx context.Context, userID, templateID string) (bool, int32, error)
	ToggleFavoriteFunc     func(ctx context.Context, userID, templateID string) (bool, int32, error)
	UpdateFunc             func(ctx context.Context, t *models.Template) error
	CountTemplatesFunc     func(ctx context.Context, filters map[string]interface{}) (int32, error)
	DeleteFunc             func(ctx context.Context, id string) error
	ListRelatedFunc        func(ctx context.Context, templateID string, tpl *models.Template, currentUserID string, limit int) ([]*models.Template, error)
	ListMoreFromAuthorFunc func(ctx context.Context, templateID, ownerID, currentUserID string, limit int) ([]*models.Template, error)
	RateTemplateFunc       func(ctx context.Context, userID, templateID string, rating int32) (int64, int32, error)
	DeleteRatingFunc       func(ctx context.Context, userID, templateID string) (int64, int32, error)
	GetRatingDistributionFunc func(ctx context.Context, templateID string) ([5]int32, error)
}

func (m *MockTemplateRepository) Create(ctx context.Context, t *models.Template) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, t)
	}
	return nil
}
func (m *MockTemplateRepository) Get(ctx context.Context, id string, currentUserID string) (*models.Template, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id, currentUserID)
	}
	return nil, nil
}
func (m *MockTemplateRepository) List(ctx context.Context, l, o int, f map[string]interface{}) ([]*models.Template, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, l, o, f)
	}
	return nil, nil
}
func (m *MockTemplateRepository) Update(ctx context.Context, t *models.Template) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, t)
	}
	return nil
}
func (m *MockTemplateRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}
func (m *MockTemplateRepository) ToggleLike(ctx context.Context, userID, templateID string) (bool, int32, error) {
	if m.ToggleLikeFunc != nil {
		return m.ToggleLikeFunc(ctx, userID, templateID)
	}
	return false, 0, nil
}
func (m *MockTemplateRepository) ToggleFavorite(ctx context.Context, userID, templateID string) (bool, int32, error) {
	if m.ToggleFavoriteFunc != nil {
		return m.ToggleFavoriteFunc(ctx, userID, templateID)
	}
	return false, 0, nil
}
func (m *MockTemplateRepository) ListCategories(ctx context.Context, filters map[string]interface{}) ([]*models.CategoryStat, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.CategoryStat), args.Error(1)
}
func (m *MockTemplateRepository) ListTags(ctx context.Context, filters map[string]interface{}) ([]*models.TagStat, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TagStat), args.Error(1)
}
func (m *MockTemplateRepository) CountTemplates(ctx context.Context, filters map[string]interface{}) (int32, error) {
	if m.CountTemplatesFunc != nil {
		return m.CountTemplatesFunc(ctx, filters)
	}
	return 0, nil
}
func (m *MockTemplateRepository) ListRelated(ctx context.Context, templateID string, tpl *models.Template, currentUserID string, limit int) ([]*models.Template, error) {
	if m.ListRelatedFunc != nil {
		return m.ListRelatedFunc(ctx, templateID, tpl, currentUserID, limit)
	}
	return nil, nil
}
func (m *MockTemplateRepository) ListMoreFromAuthor(ctx context.Context, templateID, ownerID, currentUserID string, limit int) ([]*models.Template, error) {
	if m.ListMoreFromAuthorFunc != nil {
		return m.ListMoreFromAuthorFunc(ctx, templateID, ownerID, currentUserID, limit)
	}
	return nil, nil
}
func (m *MockTemplateRepository) RateTemplate(ctx context.Context, userID, templateID string, rating int32) (int64, int32, error) {
	if m.RateTemplateFunc != nil {
		return m.RateTemplateFunc(ctx, userID, templateID, rating)
	}
	return 0, 0, nil
}
func (m *MockTemplateRepository) DeleteRating(ctx context.Context, userID, templateID string) (int64, int32, error) {
	if m.DeleteRatingFunc != nil {
		return m.DeleteRatingFunc(ctx, userID, templateID)
	}
	return 0, 0, nil
}
func (m *MockTemplateRepository) GetRatingDistribution(ctx context.Context, templateID string) ([5]int32, error) {
	if m.GetRatingDistributionFunc != nil {
		return m.GetRatingDistributionFunc(ctx, templateID)
	}
	return [5]int32{}, nil
}

// MockTemplateVersionRepository

type MockTemplateAliasRepository struct {
	CreateFunc func(ctx context.Context, alias *models.TemplateAlias) error
	GetFunc    func(ctx context.Context, templateID, aliasName string) (*models.TemplateAlias, error)
	ListFunc   func(ctx context.Context, templateID string) ([]*models.TemplateAlias, error)
	UpdateFunc func(ctx context.Context, alias *models.TemplateAlias) error
	DeleteFunc func(ctx context.Context, templateID, aliasName string) error
	UpsertFunc func(ctx context.Context, alias *models.TemplateAlias) error
}

func (m *MockTemplateAliasRepository) Create(ctx context.Context, alias *models.TemplateAlias) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, alias)
	}
	return nil
}

func (m *MockTemplateAliasRepository) Get(ctx context.Context, templateID, aliasName string) (*models.TemplateAlias, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, templateID, aliasName)
	}
	return nil, nil
}

func (m *MockTemplateAliasRepository) List(ctx context.Context, templateID string) ([]*models.TemplateAlias, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, templateID)
	}
	return nil, nil
}

func (m *MockTemplateAliasRepository) Update(ctx context.Context, alias *models.TemplateAlias) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, alias)
	}
	return nil
}

func (m *MockTemplateAliasRepository) Delete(ctx context.Context, templateID, aliasName string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, templateID, aliasName)
	}
	return nil
}

func (m *MockTemplateAliasRepository) Upsert(ctx context.Context, alias *models.TemplateAlias) error {
	if m.UpsertFunc != nil {
		return m.UpsertFunc(ctx, alias)
	}
	return nil
}

type MockTemplateVersionRepository struct {
	mock.Mock
	GetLatestFunc func(ctx context.Context, tid string) (*models.TemplateVersion, error)
	CreateFunc    func(ctx context.Context, v *models.TemplateVersion) error
}

func (m *MockTemplateVersionRepository) Create(ctx context.Context, v *models.TemplateVersion) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, v)
	}
	return nil
}
func (m *MockTemplateVersionRepository) GetLatest(ctx context.Context, tid string) (*models.TemplateVersion, error) {
	if m.GetLatestFunc != nil {
		return m.GetLatestFunc(ctx, tid)
	}
	return nil, nil
}
func (m *MockTemplateVersionRepository) List(ctx context.Context, limit, offset int, templateID string) ([]*models.TemplateVersion, error) {
	args := m.Called(ctx, limit, offset, templateID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.TemplateVersion), args.Error(1)
}

func TestCreatePrompt(t *testing.T) {
	mockPromptRepo := new(MockPromptRepository)
	mockTemplateRepo := new(MockTemplateRepository)
	mockVersionRepo := new(MockTemplateVersionRepository)

	mockAliasRepo := &MockTemplateAliasRepository{}
	svc := NewPromptService(mockPromptRepo, mockTemplateRepo, mockVersionRepo, mockAliasRepo, nil, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		req := &pb.CreatePromptRequest{
			TemplateId: "tpl_1",
			VersionId:  1,
			OwnerId:    "user_1",
			Variables:  map[string]string{"var1": "value1"},
		}

		mockPromptRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Prompt")).Return(nil)

		mockVersionRepo.On("Get", mock.Anything, int32(1)).Return(&models.TemplateVersion{
			ID:      1,
			Content: "content with ${var1}",
		}, nil)

		resp, err := svc.CreatePrompt(ContextWithUserID(context.Background(), "user_1"), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, req.OwnerId, resp.Prompt.OwnerId)

		mockPromptRepo.AssertExpectations(t)
		mockVersionRepo.AssertExpectations(t)
	})
}

func TestGetPrompt(t *testing.T) {
	mockPromptRepo := new(MockPromptRepository)
	mockTemplateRepo := new(MockTemplateRepository)
	mockVersionRepo := new(MockTemplateVersionRepository)

	mockAliasRepo := &MockTemplateAliasRepository{}
	svc := NewPromptService(mockPromptRepo, mockTemplateRepo, mockVersionRepo, mockAliasRepo, nil, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		prompt := &models.Prompt{
			ID:        "p_1",
			OwnerID:   "user_1",
			Variables: map[string]string{"var1": "v1"},
			CreatedAt: time.Now(),
		}

		mockPromptRepo.On("Get", mock.Anything, "p_1").Return(prompt, nil)

		resp, err := svc.GetPrompt(context.Background(), &pb.GetPromptRequest{Id: "p_1"})
		assert.NoError(t, err)
		assert.Equal(t, "p_1", resp.Prompt.Id)
	})
}

func TestDeletePrompt(t *testing.T) {
	mockPromptRepo := new(MockPromptRepository)
	mockTemplateRepo := new(MockTemplateRepository)
	mockVersionRepo := new(MockTemplateVersionRepository)

	mockAliasRepo := &MockTemplateAliasRepository{}
	svc := NewPromptService(mockPromptRepo, mockTemplateRepo, mockVersionRepo, mockAliasRepo, nil, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		prompt := &models.Prompt{ID: "p_1", OwnerID: "user_1"}
		mockPromptRepo.On("Get", mock.Anything, "p_1").Return(prompt, nil)
		mockPromptRepo.On("Delete", mock.Anything, "p_1").Return(nil)

		resp, err := svc.DeletePrompt(ContextWithUserID(context.Background(), "user_1"), &pb.DeletePromptRequest{Id: "p_1", OwnerId: "user_1"})
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		prompt := &models.Prompt{ID: "p_1", OwnerID: "user_1"}
		mockPromptRepo.On("Get", mock.Anything, "p_1").Return(prompt, nil)

		_, err := svc.DeletePrompt(ContextWithUserID(context.Background(), "user_2"), &pb.DeletePromptRequest{Id: "p_1", OwnerId: "user_2"})
		assert.Error(t, err)
	})

	t.Run("Unauthenticated", func(t *testing.T) {
		prompt := &models.Prompt{ID: "p_1", OwnerID: "user_1"}
		mockPromptRepo.On("Get", mock.Anything, "p_1").Return(prompt, nil)

		_, err := svc.DeletePrompt(context.Background(), &pb.DeletePromptRequest{Id: "p_1", OwnerId: "user_1"})
		assert.Error(t, err)
	})

	t.Run("AdminOverride", func(t *testing.T) {
		prompt := &models.Prompt{ID: "p_1", OwnerID: "user_1"}
		mockPromptRepo.On("Get", mock.Anything, "p_1").Return(prompt, nil)
		mockPromptRepo.On("Delete", mock.Anything, "p_1").Return(nil)

		ctx := ContextWithUserRole(ContextWithUserID(context.Background(), "admin_1"), "admin")
		resp, err := svc.DeletePrompt(ctx, &pb.DeletePromptRequest{Id: "p_1"})
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	})
}

func TestListCategories(t *testing.T) {
	mockPromptRepo := new(MockPromptRepository)
	mockTemplateRepo := new(MockTemplateRepository)
	mockVersionRepo := new(MockTemplateVersionRepository)

	mockAliasRepo := &MockTemplateAliasRepository{}
	svc := NewPromptService(mockPromptRepo, mockTemplateRepo, mockVersionRepo, mockAliasRepo, nil, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		stats := []*models.CategoryStat{
			{Name: "cat1", Count: 10},
			{Name: "cat2", Count: 5},
		}
		mockTemplateRepo.On("ListCategories", mock.Anything, mock.Anything).Return(stats, nil)

		resp, err := svc.ListCategories(context.Background(), &pb.ListCategoriesRequest{})
		assert.NoError(t, err)
		assert.Len(t, resp.Categories, 2)
		assert.Equal(t, "cat1", resp.Categories[0].Name)
		assert.Equal(t, int32(10), resp.Categories[0].Count)
	})
}

func TestListTags(t *testing.T) {
	mockPromptRepo := new(MockPromptRepository)
	mockTemplateRepo := new(MockTemplateRepository)
	mockVersionRepo := new(MockTemplateVersionRepository)

	mockAliasRepo := &MockTemplateAliasRepository{}
	svc := NewPromptService(mockPromptRepo, mockTemplateRepo, mockVersionRepo, mockAliasRepo, nil, nil, nil, nil)

	t.Run("Success", func(t *testing.T) {
		stats := []*models.TagStat{
			{Name: "tag1", Count: 20},
			{Name: "tag2", Count: 15},
		}
		mockTemplateRepo.On("ListTags", mock.Anything, mock.Anything).Return(stats, nil)

		resp, err := svc.ListTags(context.Background(), &pb.ListTagsRequest{})
		assert.NoError(t, err)
		assert.Len(t, resp.Tags, 2)
		assert.Equal(t, "tag1", resp.Tags[0].Name)
		assert.Equal(t, int32(20), resp.Tags[0].Count)
	})
}

func (m *MockTemplateVersionRepository) Get(ctx context.Context, id int32) (*models.TemplateVersion, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TemplateVersion), args.Error(1)
}

// ---- engagement notification hook tests ----

func TestToggleLikeNotificationHook(t *testing.T) {
	newSvc := func(t *testing.T, tpl *models.Template, tplErr error, liked bool, notifRepo repository.NotificationRepository, userRepo repository.UserRepository) *PromptService {
		t.Helper()
		mockTemplateRepo := &MockTemplateRepository{
			GetFunc: func(ctx context.Context, id string, currentUserID string) (*models.Template, error) {
				return tpl, tplErr
			},
			ToggleLikeFunc: func(ctx context.Context, userID, templateID string) (bool, int32, error) {
				return liked, 1, nil
			},
		}
		return NewPromptService(nil, mockTemplateRepo, nil, nil, notifRepo, userRepo, nil, nil)
	}

	tpl := &models.Template{ID: "t1", OwnerID: "owner_1", Title: "Template"}

	t.Run("LikeOnCreatesNotification", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)
		userRepo.On("GetByID", mock.Anything, "actor_1").Return(&models.User{ID: "actor_1", DisplayName: "Actor"}, nil).Once()
		notifRepo.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.RecipientID == "owner_1" && n.ActorID == "actor_1" &&
				n.TemplateID == "t1" && n.Type == "like" && n.ActorDisplayName == "Actor"
		})).Return(nil).Once()

		svc := newSvc(t, tpl, nil, true, notifRepo, userRepo)
		resp, err := svc.ToggleLikeTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleLikeRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err)
		assert.True(t, resp.IsLiked)
		notifRepo.AssertExpectations(t)
		userRepo.AssertExpectations(t)
	})

	t.Run("LikeOffDoesNotCreateNotification", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)

		svc := newSvc(t, tpl, nil, false, notifRepo, userRepo)
		resp, err := svc.ToggleLikeTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleLikeRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err)
		assert.False(t, resp.IsLiked)
		notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("SelfLikeSuppressed", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)

		svc := newSvc(t, tpl, nil, true, notifRepo, userRepo)
		_, err := svc.ToggleLikeTemplate(
			ContextWithUserID(context.Background(), "owner_1"),
			&pb.ToggleLikeRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err)
		notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		userRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
	})

	t.Run("TemplateLoadErrorReturnsNotFound", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)

		svc := newSvc(t, nil, errors.New("template gone"), true, notifRepo, userRepo)
		_, err := svc.ToggleLikeTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleLikeRequest{TemplateId: "t1"},
		)
		assert.Error(t, err, "hidden/missing templates must 404 before toggling")
		assert.Equal(t, codes.NotFound, status.Code(err))
		notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("CreateErrorSwallowed", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)
		userRepo.On("GetByID", mock.Anything, "actor_1").Return(&models.User{ID: "actor_1", DisplayName: "Actor"}, nil).Once()
		notifRepo.On("Create", mock.Anything, mock.Anything).Return(errors.New("insert failed")).Once()

		svc := newSvc(t, tpl, nil, true, notifRepo, userRepo)
		resp, err := svc.ToggleLikeTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleLikeRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err, "notification create failure must not fail the like")
		assert.True(t, resp.IsLiked)
		notifRepo.AssertExpectations(t)
	})

	t.Run("ActorLoadErrorSwallowed", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)
		userRepo.On("GetByID", mock.Anything, "actor_1").Return(nil, errors.New("user gone")).Once()

		svc := newSvc(t, tpl, nil, true, notifRepo, userRepo)
		resp, err := svc.ToggleLikeTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleLikeRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err)
		assert.True(t, resp.IsLiked)
		notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		userRepo.AssertExpectations(t)
	})

	t.Run("EmptyDisplayNameFallsBackToID", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)
		userRepo.On("GetByID", mock.Anything, "actor_1").Return(&models.User{ID: "actor_1"}, nil).Once()
		notifRepo.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.ActorDisplayName == "actor_1"
		})).Return(nil).Once()

		svc := newSvc(t, tpl, nil, true, notifRepo, userRepo)
		_, err := svc.ToggleLikeTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleLikeRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err)
		notifRepo.AssertExpectations(t)
		userRepo.AssertExpectations(t)
	})
}

func TestToggleFavoriteNotificationHook(t *testing.T) {
	tpl := &models.Template{ID: "t1", OwnerID: "owner_1"}

	t.Run("FavoriteOnCreatesNotification", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)
		userRepo.On("GetByID", mock.Anything, "actor_1").Return(&models.User{ID: "actor_1", DisplayName: "Actor"}, nil).Once()
		notifRepo.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.Type == "favorite" && n.RecipientID == "owner_1"
		})).Return(nil).Once()

		mockTemplateRepo := &MockTemplateRepository{
			GetFunc: func(ctx context.Context, id string, currentUserID string) (*models.Template, error) {
				return tpl, nil
			},
			ToggleFavoriteFunc: func(ctx context.Context, userID, templateID string) (bool, int32, error) {
				return true, 1, nil
			},
		}
		svc := NewPromptService(nil, mockTemplateRepo, nil, nil, notifRepo, userRepo, nil, nil)
		resp, err := svc.ToggleFavoriteTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleFavoriteRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err)
		assert.True(t, resp.IsFavorited)
		notifRepo.AssertExpectations(t)
		userRepo.AssertExpectations(t)
	})

	t.Run("FavoriteOffDoesNotCreateNotification", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)

		mockTemplateRepo := &MockTemplateRepository{
			ToggleFavoriteFunc: func(ctx context.Context, userID, templateID string) (bool, int32, error) {
				return false, 0, nil
			},
		}
		svc := NewPromptService(nil, mockTemplateRepo, nil, nil, notifRepo, userRepo, nil, nil)
		resp, err := svc.ToggleFavoriteTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			&pb.ToggleFavoriteRequest{TemplateId: "t1"},
		)
		assert.NoError(t, err)
		assert.False(t, resp.IsFavorited)
		notifRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})
}

func TestForkNotificationHook(t *testing.T) {
	t.Run("ForkCreatesNotificationForSourceOwner", func(t *testing.T) {
		notifRepo := new(MockNotificationRepository)
		userRepo := new(MockUserRepository)
		userRepo.On("GetByID", mock.Anything, "actor_1").Return(&models.User{ID: "actor_1", DisplayName: "Actor"}, nil).Once()
		notifRepo.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.Type == "fork" && n.RecipientID == "owner_1" && n.TemplateID == "t1"
		})).Return(nil).Once()

		mockTemplateRepo := &MockTemplateRepository{
			GetFunc: func(ctx context.Context, id string, currentUserID string) (*models.Template, error) {
				return &models.Template{ID: "t1", OwnerID: "owner_1"}, nil
			},
		}
		mockVersionRepo := &MockTemplateVersionRepository{
			GetLatestFunc: func(ctx context.Context, tid string) (*models.TemplateVersion, error) {
				return &models.TemplateVersion{ID: 1, Content: "c"}, nil
			},
		}
		mockVersionRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.TemplateVersion")).Return(nil)

		svc := NewPromptService(nil, mockTemplateRepo, mockVersionRepo, nil, notifRepo, userRepo, nil, nil)
		_, err := svc.ForkTemplate(
			ContextWithUserID(context.Background(), "actor_1"),
			"t1",
		)
		assert.NoError(t, err)
		notifRepo.AssertExpectations(t)
		userRepo.AssertExpectations(t)
	})
}

func TestTemplateMutationAuthz(t *testing.T) {
	mockTemplateRepo := new(MockTemplateRepository)
	svc := NewPromptService(new(MockPromptRepository), mockTemplateRepo, new(MockTemplateVersionRepository), &MockTemplateAliasRepository{}, nil, nil, nil, nil)

	t.Run("UpdateTemplateNonOwner", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: "t_1", OwnerID: "owner_1"}, nil
		}
		_, err := svc.UpdateTemplate(ContextWithUserID(context.Background(), "user_2"), &pb.UpdateTemplateRequest{TemplateId: "t_1"})
		assert.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("UpdateTemplateUnauthenticated", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: "t_1", OwnerID: "owner_1"}, nil
		}
		_, err := svc.UpdateTemplate(context.Background(), &pb.UpdateTemplateRequest{TemplateId: "t_1"})
		assert.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("UpdateTemplateAdminOverride", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: "t_1", OwnerID: "owner_1"}, nil
		}
		mockTemplateRepo.UpdateFunc = func(ctx context.Context, tpl *models.Template) error {
			return nil
		}
		ctx := ContextWithUserRole(ContextWithUserID(context.Background(), "admin_1"), "admin")
		_, err := svc.UpdateTemplate(ctx, &pb.UpdateTemplateRequest{TemplateId: "t_1", Content: "c"})
		assert.NoError(t, err)
	})

	t.Run("DeleteTemplateNonOwner", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: "t_1", OwnerID: "owner_1"}, nil
		}
		_, err := svc.DeleteTemplate(ContextWithUserID(context.Background(), "user_2"), &pb.DeleteTemplateRequest{Id: "t_1"})
		assert.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

func TestAliasMutationAuthz(t *testing.T) {
	mockTemplateRepo := new(MockTemplateRepository)
	aliasRepo := &MockTemplateAliasRepository{}
	svc := NewPromptService(new(MockPromptRepository), mockTemplateRepo, new(MockTemplateVersionRepository), aliasRepo, nil, nil, nil, nil)

	t.Run("CreateAliasNonOwner", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: "t_1", OwnerID: "owner_1"}, nil
		}
		_, err := svc.CreateAlias(ContextWithUserID(context.Background(), "user_2"), &pb.CreateAliasRequest{TemplateId: "t_1", AliasName: "a"})
		assert.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("CreateAliasUnauthenticated", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: "t_1", OwnerID: "owner_1"}, nil
		}
		_, err := svc.CreateAlias(context.Background(), &pb.CreateAliasRequest{TemplateId: "t_1", AliasName: "a"})
		assert.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("CreateAliasMissingTemplate", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return nil, nil
		}
		_, err := svc.CreateAlias(ContextWithUserID(context.Background(), "owner_1"), &pb.CreateAliasRequest{TemplateId: "missing", AliasName: "a"})
		assert.Error(t, err)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("UpdateAliasOwner", func(t *testing.T) {
		mockTemplateRepo.GetFunc = func(ctx context.Context, id, currentUserID string) (*models.Template, error) {
			return &models.Template{ID: "t_1", OwnerID: "owner_1"}, nil
		}
		aliasRepo.UpdateFunc = func(ctx context.Context, alias *models.TemplateAlias) error {
			return nil
		}
		_, err := svc.UpdateAlias(ContextWithUserID(context.Background(), "owner_1"), &pb.UpdateAliasRequest{TemplateId: "t_1", AliasName: "a", VersionId: 2})
		assert.NoError(t, err)
	})
}

func TestListPromptsAuthz(t *testing.T) {
	mockPromptRepo := new(MockPromptRepository)
	svc := NewPromptService(mockPromptRepo, new(MockTemplateRepository), new(MockTemplateVersionRepository), &MockTemplateAliasRepository{}, nil, nil, nil, nil)

	t.Run("OwnerListOwnPrompts", func(t *testing.T) {
		mockPromptRepo.On("List", mock.Anything, 10, 0, map[string]interface{}{"owner_id": "user_1"}).Return([]*models.Prompt{}, nil).Once()
		_, err := svc.ListPrompts(ContextWithUserID(context.Background(), "user_1"), &pb.ListPromptsRequest{OwnerId: "user_1"})
		assert.NoError(t, err)
	})

	t.Run("NonOwnerList", func(t *testing.T) {
		_, err := svc.ListPrompts(ContextWithUserID(context.Background(), "user_2"), &pb.ListPromptsRequest{OwnerId: "user_1"})
		assert.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("AnonymousListWithOwner", func(t *testing.T) {
		_, err := svc.ListPrompts(context.Background(), &pb.ListPromptsRequest{OwnerId: "user_1"})
		assert.Error(t, err)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("AdminListAny", func(t *testing.T) {
		mockPromptRepo.On("List", mock.Anything, 10, 0, map[string]interface{}{"owner_id": "user_1"}).Return([]*models.Prompt{}, nil).Once()
		ctx := ContextWithUserRole(ContextWithUserID(context.Background(), "admin_1"), "admin")
		_, err := svc.ListPrompts(ctx, &pb.ListPromptsRequest{OwnerId: "user_1"})
		assert.NoError(t, err)
	})
}
