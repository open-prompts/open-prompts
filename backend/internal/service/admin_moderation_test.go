package service

import (
	"context"
	"strconv"
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

func newAdminModerationService(t *testing.T) (*AdminService, *MockUserRepository, *MockAdminRepository, *MockNotificationRepository, *MockReportRepository, *MockRedisStore) {
	t.Helper()
	svc, mockUser, mockAdmin, mockRedis, _ := newAdminTestService(t)
	notif := new(MockNotificationRepository)
	svc.SetNotificationRepo(notif)
	report := new(MockReportRepository)
	svc.SetReportRepo(report)
	return svc, mockUser, mockAdmin, notif, report, mockRedis
}

func moderationTemplate(ownerID string) *models.AdminTemplate {
	return &models.AdminTemplate{
		ID:               "tpl1",
		OwnerID:          ownerID,
		Title:            "My Template",
		Visibility:       "public",
		ModerationStatus: "normal",
	}
}

func TestUnpublishTemplate(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminModerationService(t)
		_, err := svc.UnpublishTemplate(context.Background(), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "spam content"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("ReasonTooShort", func(t *testing.T) {
		svc, mockUser, _, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		_, err := svc.UnpublishTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "ab"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(nil, nil, repository.ErrTemplateNotFound).Once()
		_, err := svc.UnpublishTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "spam content"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("SelfGuard", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("admin1"), []*models.AdminTemplateVersion{}, nil).Once()
		_, err := svc.UnpublishTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "spam content"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("InvalidTransition", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		tpl := moderationTemplate("alice")
		tpl.ModerationStatus = "soft_deleted"
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(tpl, []*models.AdminTemplateVersion{}, nil).Once()
		_, err := svc.UnpublishTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "spam content"})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, notif, report, mockRedis := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("alice"), []*models.AdminTemplateVersion{}, nil).Once()
		mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("", nil).Once()
		mockRedis.On("Set", mock.Anything, "mod_action_rate:admin1", mock.Anything, mock.Anything).Return(nil).Once()
		mockAdmin.On("SetModerationState", mock.Anything, "tpl1", "unpublished_by_admin", "admin1", "spam content").Return(nil).Once()
		mockAdmin.On("InsertModerationAction", mock.Anything, mock.Anything).Return("act1", nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		report.On("AutoResolveItem", mock.Anything, "template", "tpl1", "admin1").Return(nil).Once()
		notif.On("Create", mock.Anything, mock.Anything).Return(nil).Once()

		resp, err := svc.UnpublishTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "spam content"})
		assert.NoError(t, err)
		assert.Equal(t, "tpl1", resp.TemplateId)
		assert.Equal(t, "unpublished_by_admin", resp.ModerationStatus)
	})
}

func TestLockTemplate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, notif, report, mockRedis := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("alice"), []*models.AdminTemplateVersion{}, nil).Once()
		mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("", nil).Once()
		mockRedis.On("Set", mock.Anything, "mod_action_rate:admin1", mock.Anything, mock.Anything).Return(nil).Once()
		mockAdmin.On("SetModerationState", mock.Anything, "tpl1", "locked", "admin1", "under review").Return(nil).Once()
		mockAdmin.On("InsertModerationAction", mock.Anything, mock.Anything).Return("act1", nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		// Lock does NOT auto-resolve reports.
		notif.On("Create", mock.Anything, mock.Anything).Return(nil).Once()

		resp, err := svc.LockTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "under review"})
		assert.NoError(t, err)
		assert.Equal(t, "locked", resp.ModerationStatus)
		report.AssertNotCalled(t, "AutoResolveItem", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestSoftDeleteTemplate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, notif, report, mockRedis := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("alice"), []*models.AdminTemplateVersion{}, nil).Once()
		mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("", nil).Once()
		mockRedis.On("Set", mock.Anything, "mod_action_rate:admin1", mock.Anything, mock.Anything).Return(nil).Once()
		mockAdmin.On("SetModerationState", mock.Anything, "tpl1", "soft_deleted", "admin1", "removed").Return(nil).Once()
		mockAdmin.On("SetSoftDeleteTimestamp", mock.Anything, "tpl1").Return(nil).Once()
		mockAdmin.On("InsertModerationAction", mock.Anything, mock.Anything).Return("act1", nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		report.On("AutoResolveItem", mock.Anything, "template", "tpl1", "admin1").Return(nil).Once()
		notif.On("Create", mock.Anything, mock.Anything).Return(nil).Once()

		resp, err := svc.SoftDeleteTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "removed"})
		assert.NoError(t, err)
		assert.Equal(t, "soft_deleted", resp.ModerationStatus)
	})
}

func TestSoftDeleteTemplateErrors(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminModerationService(t)
		_, err := svc.SoftDeleteTemplate(context.Background(), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "removed"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(nil, nil, repository.ErrTemplateNotFound).Once()
		_, err := svc.SoftDeleteTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "removed"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("SelfGuard", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("admin1"), []*models.AdminTemplateVersion{}, nil).Once()
		_, err := svc.SoftDeleteTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "removed"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("InvalidTransition", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		tpl := moderationTemplate("alice")
		tpl.ModerationStatus = "soft_deleted"
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(tpl, []*models.AdminTemplateVersion{}, nil).Once()
		_, err := svc.SoftDeleteTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "removed"})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("RateLimitExceeded", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, mockRedis := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("alice"), []*models.AdminTemplateVersion{}, nil).Once()
		mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("10:"+strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10), nil).Once()
		_, err := svc.SoftDeleteTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "removed"})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})
}

func TestRestoreTemplate(t *testing.T) {
	t.Run("NothingToRestore", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("alice"), []*models.AdminTemplateVersion{}, nil).Once()
		_, err := svc.RestoreTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "mistake"})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, notif, _, mockRedis := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		tpl := moderationTemplate("alice")
		tpl.ModerationStatus = "soft_deleted"
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(tpl, []*models.AdminTemplateVersion{}, nil).Once()
		mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("", nil).Once()
		mockRedis.On("Set", mock.Anything, "mod_action_rate:admin1", mock.Anything, mock.Anything).Return(nil).Once()
		mockAdmin.On("SetModerationState", mock.Anything, "tpl1", "normal", "admin1", "mistake").Return(nil).Once()
		mockAdmin.On("ClearSoftDeleteTimestamp", mock.Anything, "tpl1").Return(nil).Once()
		mockAdmin.On("MarkModerationActionSuperseded", mock.Anything, "tpl1").Return(nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		notif.On("Create", mock.Anything, mock.Anything).Return(nil).Once()

		resp, err := svc.RestoreTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "mistake"})
		assert.NoError(t, err)
		assert.Equal(t, "normal", resp.ModerationStatus)
	})
}

func TestRestoreTemplateErrors(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminModerationService(t)
		_, err := svc.RestoreTemplate(context.Background(), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "mistake"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(nil, nil, repository.ErrTemplateNotFound).Once()
		_, err := svc.RestoreTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "mistake"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("SelfGuard", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, _ := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		tpl := moderationTemplate("admin1")
		tpl.ModerationStatus = "soft_deleted"
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(tpl, []*models.AdminTemplateVersion{}, nil).Once()
		_, err := svc.RestoreTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "mistake"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("RateLimitExceeded", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, mockRedis := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		tpl := moderationTemplate("alice")
		tpl.ModerationStatus = "soft_deleted"
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(tpl, []*models.AdminTemplateVersion{}, nil).Once()
		mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("10:"+strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10), nil).Once()
		_, err := svc.RestoreTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "mistake"})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})
}

func TestModerationRateLimit(t *testing.T) {
	t.Run("Exceeded", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _, mockRedis := newAdminModerationService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(moderationTemplate("alice"), []*models.AdminTemplateVersion{}, nil).Once()
		// Simulate an already-saturated window.
		mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("10:"+strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10), nil).Once()
		_, err := svc.UnpublishTemplate(adminCtx("admin1"), &pb.ModerationActionRequest{TemplateId: "tpl1", Reason: "spam content"})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})
}

func TestValidModerationTransition(t *testing.T) {
	cases := []struct {
		from, action string
		want         bool
	}{
		{"normal", "unpublish", true},
		{"normal", "lock", true},
		{"normal", "soft_delete", true},
		{"normal", "restore", false},
		{"locked", "unpublish", true},
		{"locked", "lock", false},
		{"locked", "soft_delete", true},
		{"locked", "restore", true},
		{"unpublished_by_admin", "unpublish", true},
		{"unpublished_by_admin", "lock", true},
		{"unpublished_by_admin", "soft_delete", true},
		{"unpublished_by_admin", "restore", true},
		{"soft_deleted", "unpublish", false},
		{"soft_deleted", "lock", false},
		{"soft_deleted", "soft_delete", false},
		{"soft_deleted", "restore", true},
		{"in_review", "restore", false},
		{"rejected", "restore", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, validModerationTransition(c.from, c.action), "%s -> %s", c.from, c.action)
	}
}

func TestNotifyModeration(t *testing.T) {
	t.Run("SelfSuppressed", func(t *testing.T) {
		svc, _, _, notif, _, _ := newAdminModerationService(t)
		svc.notifyModeration(context.Background(), makeAdminUser("admin1"), "admin1", "tpl1", "unpublish", "reason")
		notif.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("NilRepo", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminModerationService(t)
		svc.SetNotificationRepo(nil)
		svc.notifyModeration(context.Background(), makeAdminUser("admin1"), "alice", "tpl1", "unpublish", "reason")
	})

	t.Run("CreatesNotification", func(t *testing.T) {
		svc, _, _, notif, _, _ := newAdminModerationService(t)
		notif.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.RecipientID == "alice" && n.Type == "moderation" && n.ActorDisplayName == "unpublish" && n.Snippet == "reason"
		})).Return(nil).Once()
		svc.notifyModeration(context.Background(), makeAdminUser("admin1"), "alice", "tpl1", "unpublish", "reason")
		notif.AssertExpectations(t)
	})

	t.Run("TruncatesLongReason", func(t *testing.T) {
		svc, _, _, notif, _, _ := newAdminModerationService(t)
		long := string(make([]rune, 200))
		notif.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return len([]rune(n.Snippet)) <= 141
		})).Return(nil).Once()
		svc.notifyModeration(context.Background(), makeAdminUser("admin1"), "alice", "tpl1", "lock", long)
		notif.AssertExpectations(t)
	})

	t.Run("CreateErrorLogged", func(t *testing.T) {
		svc, _, _, notif, _, _ := newAdminModerationService(t)
		notif.On("Create", mock.Anything, mock.Anything).Return(assert.AnError).Once()
		svc.notifyModeration(context.Background(), makeAdminUser("admin1"), "alice", "tpl1", "unpublish", "reason")
		notif.AssertExpectations(t)
	})
}

func TestModerationRateLimitCounter(t *testing.T) {
	svc, _, _, _, _, mockRedis := newAdminModerationService(t)
	mockRedis.On("Get", mock.Anything, "mod_action_rate:admin1").Return("", nil).Once()
	mockRedis.On("Set", mock.Anything, "mod_action_rate:admin1", mock.Anything, mock.Anything).Return(nil).Once()
	retryAfter, ok := svc.moderationRateLimit(context.Background(), "admin1")
	assert.True(t, ok)
	assert.Equal(t, 0, retryAfter)
}