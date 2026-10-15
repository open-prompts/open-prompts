package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"
)

// newWarningsAdminService builds an AdminService wired with a review repo
// (default taxonomy) and a notification repo for warning-change tests.
func newWarningsAdminService(t *testing.T) (*AdminService, *MockUserRepository, *MockAdminRepository, *MockNotificationRepository) {
	t.Helper()
	svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
	review := &MockReviewRepository{
		GetSettingFn: func(ctx context.Context, key string) (string, error) {
			if key == settingWarningTaxonomy {
				return `["mature","violence","profanity","sensitive"]`, nil
			}
			return "", repository.ErrSubmissionNotFound
		},
	}
	svc.SetReviewRepo(review)
	notif := new(MockNotificationRepository)
	svc.SetNotificationRepo(notif)
	return svc, mockUser, mockAdmin, notif
}

func TestUpdateTemplateWarnings(t *testing.T) {
	admin := makeAdminUser("admin_1")
	owner := makeRegularUser("owner_1")

	t.Run("add warning with reason", func(t *testing.T) {
		svc, mockUser, mockAdmin, notif := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl_1").Return(&models.AdminTemplate{
			ID:              "tpl_1",
			OwnerID:         owner.ID,
			ContentWarnings: []string{"mature"},
			LockedWarnings:  nil,
		}, []*models.AdminTemplateVersion{}, nil)
		mockAdmin.On("UpdateTemplateWarnings", mock.Anything, "tpl_1",
			[]string{"mature", "violence"}, []string{"violence"}).Return(nil)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.AnythingOfType("*models.AdminAuditLog")).Return(nil)
		notif.On("Create", mock.Anything, mock.AnythingOfType("*models.Notification")).Return(nil)

		resp, err := svc.UpdateTemplateWarnings(adminCtx(admin.ID), &pb.UpdateTemplateWarningsRequest{
			TemplateId: "tpl_1",
			Add:        []string{"violence"},
			Reason:     "adds graphic violence",
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"mature", "violence"}, resp.ContentWarnings)
		assert.Equal(t, []string{"violence"}, resp.LockedWarnings)
		mockAdmin.AssertExpectations(t)
		notif.AssertExpectations(t)
	})

	t.Run("remove warning with reason", func(t *testing.T) {
		svc, mockUser, mockAdmin, notif := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl_1").Return(&models.AdminTemplate{
			ID:              "tpl_1",
			OwnerID:         owner.ID,
			ContentWarnings: []string{"mature", "violence"},
			LockedWarnings:  []string{"violence"},
		}, []*models.AdminTemplateVersion{}, nil)
		mockAdmin.On("UpdateTemplateWarnings", mock.Anything, "tpl_1",
			[]string{"mature"}, mock.MatchedBy(func(l []string) bool { return len(l) == 0 })).Return(nil)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.AnythingOfType("*models.AdminAuditLog")).Return(nil)
		notif.On("Create", mock.Anything, mock.AnythingOfType("*models.Notification")).Return(nil)

		resp, err := svc.UpdateTemplateWarnings(adminCtx(admin.ID), &pb.UpdateTemplateWarningsRequest{
			TemplateId: "tpl_1",
			Remove:     []string{"violence"},
			Reason:     "no longer warranted",
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"mature"}, resp.ContentWarnings)
		assert.Empty(t, resp.LockedWarnings)
		mockAdmin.AssertExpectations(t)
		notif.AssertExpectations(t)
	})

	t.Run("reason too short -> 400", func(t *testing.T) {
		svc, mockUser, _, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		_, err := svc.UpdateTemplateWarnings(adminCtx(admin.ID), &pb.UpdateTemplateWarningsRequest{
			TemplateId: "tpl_1",
			Add:        []string{"violence"},
			Reason:     "ab",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "reason")
	})

	t.Run("unknown taxonomy key -> 400", func(t *testing.T) {
		svc, mockUser, _, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		_, err := svc.UpdateTemplateWarnings(adminCtx(admin.ID), &pb.UpdateTemplateWarningsRequest{
			TemplateId: "tpl_1",
			Add:        []string{"bogus"},
			Reason:     "adds bogus warning",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown content warning")
	})

	t.Run("template not found -> 404", func(t *testing.T) {
		svc, mockUser, mockAdmin, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "missing").Return(nil, nil, repository.ErrTemplateNotFound)
		_, err := svc.UpdateTemplateWarnings(adminCtx(admin.ID), &pb.UpdateTemplateWarningsRequest{
			TemplateId: "missing",
			Add:        []string{"violence"},
			Reason:     "adds graphic violence",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("self-action suppressed (no notification)", func(t *testing.T) {
		svc, mockUser, mockAdmin, notif := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		// Admin owns the template -> self-suppression.
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl_1").Return(&models.AdminTemplate{
			ID:              "tpl_1",
			OwnerID:         admin.ID,
			ContentWarnings: []string{},
			LockedWarnings:  nil,
		}, []*models.AdminTemplateVersion{}, nil)
		mockAdmin.On("UpdateTemplateWarnings", mock.Anything, "tpl_1",
			[]string{"mature"}, []string{"mature"}).Return(nil)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.AnythingOfType("*models.AdminAuditLog")).Return(nil)
		// No notification expected.

		resp, err := svc.UpdateTemplateWarnings(adminCtx(admin.ID), &pb.UpdateTemplateWarningsRequest{
			TemplateId: "tpl_1",
			Add:        []string{"mature"},
			Reason:     "adds mature themes",
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"mature"}, resp.ContentWarnings)
		notif.AssertNotCalled(t, "Create", mock.Anything, mock.AnythingOfType("*models.Notification"))
	})
}

func TestListWarningHistory(t *testing.T) {
	admin := makeAdminUser("admin_1")

	t.Run("returns paginated history", func(t *testing.T) {
		svc, mockUser, mockAdmin, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		mockAdmin.On("ListWarningHistory", mock.Anything, "tpl_1", 20, 0).Return([]*models.WarningHistoryEntry{
			{ID: "a1", ActorID: admin.ID, ActorEmail: admin.Email, Action: "template.warning_add", Warnings: []string{"violence"}, Reason: `["violence"]`},
		}, int64(1), nil)

		resp, err := svc.ListWarningHistory(adminCtx(admin.ID), &pb.ListWarningHistoryRequest{TemplateId: "tpl_1"})
		assert.NoError(t, err)
		assert.Equal(t, int64(1), resp.Total)
		assert.Len(t, resp.Entries, 1)
		assert.Equal(t, "template.warning_add", resp.Entries[0].Action)
		assert.Equal(t, []string{"violence"}, resp.Entries[0].Warnings)
		mockAdmin.AssertExpectations(t)
	})

	t.Run("missing template id -> 400", func(t *testing.T) {
		svc, mockUser, _, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		_, err := svc.ListWarningHistory(adminCtx(admin.ID), &pb.ListWarningHistoryRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template id")
	})
}

func TestAdminListTemplatesWarningFilter(t *testing.T) {
	admin := makeAdminUser("admin_1")

	t.Run("valid taxonomy key accepted", func(t *testing.T) {
		svc, mockUser, mockAdmin, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		mockAdmin.On("ListTemplates", mock.Anything, mock.MatchedBy(func(f repository.TemplateFilters) bool {
			return f.Warning == "mature"
		})).Return([]*models.AdminTemplate{}, int64(0), nil)

		resp, err := svc.ListTemplates(adminCtx(admin.ID), &pb.ListAdminTemplatesRequest{Warning: "mature"})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		mockAdmin.AssertExpectations(t)
	})

	t.Run("invalid taxonomy key -> 400", func(t *testing.T) {
		svc, mockUser, _, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		_, err := svc.ListTemplates(adminCtx(admin.ID), &pb.ListAdminTemplatesRequest{Warning: "bogus"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid warning filter")
	})

	t.Run("none accepted", func(t *testing.T) {
		svc, mockUser, mockAdmin, _ := newWarningsAdminService(t)
		expectAdminAuth(mockUser, admin)
		mockAdmin.On("ListTemplates", mock.Anything, mock.MatchedBy(func(f repository.TemplateFilters) bool {
			return f.Warning == "none"
		})).Return([]*models.AdminTemplate{}, int64(0), nil)

		resp, err := svc.ListTemplates(adminCtx(admin.ID), &pb.ListAdminTemplatesRequest{Warning: "none"})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		mockAdmin.AssertExpectations(t)
	})
}

func TestUnionMinus(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, unionMinus([]string{"a"}, []string{"b"}, nil))
	assert.Equal(t, []string{"a"}, unionMinus([]string{"a", "b"}, nil, []string{"b"}))
	assert.Equal(t, []string{"a"}, unionMinus([]string{"a", "b"}, []string{"b"}, []string{"b"}))
	assert.Equal(t, []string{"a", "b"}, unionMinus(nil, []string{"a", "b"}, nil))
	assert.Empty(t, unionMinus([]string{"a"}, nil, []string{"a"}))
}

func TestAdminTaxonomyFallback(t *testing.T) {
	svc, _, _, _ := newWarningsAdminService(t)
	// No review repo wired -> default taxonomy.
	svc.ReviewRepo = nil
	assert.Equal(t, defaultWarningTaxonomy, svc.taxonomy(context.Background()))
}

func TestNotifyWarningChangeSelfSuppressed(t *testing.T) {
	admin := makeAdminUser("admin_1")
	svc, _, _, notif := newWarningsAdminService(t)
	// Admin owns the template -> self-suppression, no notification.
	svc.notifyWarningChange(context.Background(), admin, admin.ID, "tpl_1", []string{"mature"}, nil)
	notif.AssertNotCalled(t, "Create", mock.Anything, mock.AnythingOfType("*models.Notification"))
}

func TestNotifyWarningChangeNilRepo(t *testing.T) {
	admin := makeAdminUser("admin_1")
	owner := makeRegularUser("owner_1")
	svc, _, _, _ := newWarningsAdminService(t)
	// No notification repo -> no-op.
	svc.NotificationRepo = nil
	svc.notifyWarningChange(context.Background(), admin, owner.ID, "tpl_1", []string{"mature"}, nil)
}