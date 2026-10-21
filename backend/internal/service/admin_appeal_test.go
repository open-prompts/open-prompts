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

func newAdminAppealService(t *testing.T) (*AdminService, *MockUserRepository, *MockAdminRepository, *MockAppealRepository, *MockNotificationRepository, *MockRedisStore) {
	t.Helper()
	svc, mockUser, mockAdmin, mockRedis, _ := newAdminTestService(t)
	appealRepo := new(MockAppealRepository)
	svc.SetAppealRepo(appealRepo)
	notif := new(MockNotificationRepository)
	svc.SetNotificationRepo(notif)
	return svc, mockUser, mockAdmin, appealRepo, notif, mockRedis
}

func adminAppealDetail(appealID, state, actionActorID string) (*models.Appeal, *models.ModerationAction) {
	appeal := &models.Appeal{
		ID:                 appealID,
		ModerationActionID: "act1",
		AppellantID:        "alice",
		Statement:          "This decision is wrong because the links point to my company's product documentation, not off-platform promotion, and the template follows the community guidelines.",
		State:              state,
		CreatedAt:          time.Now().Add(-time.Hour),
	}
	action := &models.ModerationAction{
		ID:         "act1",
		TemplateID: "tpl1",
		ActionType: "soft_delete",
		ActorID:    actionActorID,
		ActorEmail: actionActorID + "@example.com",
		Reason:     "spam",
		Status:     "active",
		CreatedAt:  time.Now().Add(-2 * time.Hour),
	}
	return appeal, action
}

func TestListAppealQueue(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminAppealService(t)
		_, err := svc.ListAppealQueue(context.Background(), &pb.ListAppealQueueRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("InvalidState", func(t *testing.T) {
		svc, mockUser, _, _, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		_, err := svc.ListAppealQueue(adminCtx("admin1"), &pb.ListAppealQueueRequest{State: "bogus"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, mockUser, _, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appealRepo.On("ListQueue", mock.Anything, "open", 1, 20).Return([]models.Appeal{
			{ID: "a1", State: "open", TemplateID: "t1", ActionType: "lock", TemplateTitle: "T", Statement: "This is a statement that is long enough to be excerpted down to eighty characters or so for the queue display.", AppellantDisplayName: "Alice", CreatedAt: time.Now()},
		}, int64(1), nil).Once()
		appealRepo.On("CountByState", mock.Anything).Return(1, 2, 3, nil).Once()
		resp, err := svc.ListAppealQueue(adminCtx("admin1"), &pb.ListAppealQueueRequest{State: "open"})
		assert.NoError(t, err)
		assert.Len(t, resp.Entries, 1)
		assert.Equal(t, int32(1), resp.OpenCount)
		assert.Equal(t, int32(2), resp.UnderReviewCount)
		assert.Equal(t, int32(3), resp.ResolvedCount)
	})
}

func TestGetAppealAdmin(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminAppealService(t)
		_, err := svc.GetAppeal(context.Background(), &pb.GetAdminAppealRequest{AppealId: "a1"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, mockUser, _, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(nil, repository.ErrAppealNotFound).Once()
		_, err := svc.GetAppeal(adminCtx("admin1"), &pb.GetAdminAppealRequest{AppealId: "a1"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin2")
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(&models.AdminTemplate{Title: "My Template"}, []*models.AdminTemplateVersion{}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", DisplayName: "Alice"}, nil).Once()
		mockAdmin.On("ListAuditByTarget", mock.Anything, "tpl1", 50).Return([]*models.AdminAuditLog{}, nil).Once()
		resp, err := svc.GetAppeal(adminCtx("admin1"), &pb.GetAdminAppealRequest{AppealId: "a1"})
		assert.NoError(t, err)
		assert.Equal(t, "a1", resp.Id)
		assert.Equal(t, "My Template", resp.TemplateTitle)
		assert.Equal(t, "Alice", resp.AppellantDisplayName)
	})
}

func TestReviewAppeal(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminAppealService(t)
		_, err := svc.ReviewAppeal(context.Background(), &pb.ReviewAppealRequest{AppealId: "a1"})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NotOpen", func(t *testing.T) {
		svc, mockUser, _, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, _ := adminAppealDetail("a1", "under_review", "admin2")
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		_, err := svc.ReviewAppeal(adminCtx("admin1"), &pb.ReviewAppealRequest{AppealId: "a1"})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin2")
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("SetState", mock.Anything, "a1", "under_review").Return(nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(&models.AdminTemplate{Title: "T"}, []*models.AdminTemplateVersion{}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", DisplayName: "Alice"}, nil).Once()
		mockAdmin.On("ListAuditByTarget", mock.Anything, "tpl1", 50).Return([]*models.AdminAuditLog{}, nil).Once()
		resp, err := svc.ReviewAppeal(adminCtx("admin1"), &pb.ReviewAppealRequest{AppealId: "a1"})
		assert.NoError(t, err)
		assert.Equal(t, "under_review", resp.State)
	})
}

func TestResolveAppeal(t *testing.T) {
	note := "Reviewed the links; they are legitimate product documentation. The decision is reversed."
	statement := "This decision is wrong because the links point to my company's product documentation, not off-platform promotion, and the template follows the community guidelines."

	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _, _ := newAdminAppealService(t)
		_, err := svc.ResolveAppeal(context.Background(), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: note})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("InvalidOutcome", func(t *testing.T) {
		svc, mockUser, _, _, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		_, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "bogus", Note: note})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("NoteTooShort", func(t *testing.T) {
		svc, mockUser, _, _, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		_, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: "short"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("SelfGuard", func(t *testing.T) {
		svc, mockUser, _, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin1") // actor == resolver
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		_, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: note})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("AlreadyResolved", func(t *testing.T) {
		svc, mockUser, _, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "resolved_upheld", "admin2")
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		_, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: note})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("SupersededAction", func(t *testing.T) {
		svc, mockUser, _, appealRepo, _, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin2")
		action.Status = "superseded"
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		_, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: note})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("SuccessUpheld", func(t *testing.T) {
		svc, mockUser, mockAdmin, appealRepo, notif, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin2")
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		appealRepo.On("ResolveAppeal", mock.Anything, "a1", "upheld", note, "admin1").Return(nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		notif.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.Type == "appeal_resolved" && n.RecipientID == "alice"
		})).Return(nil).Once()
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(&models.AdminTemplate{Title: "T"}, []*models.AdminTemplateVersion{}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", DisplayName: "Alice"}, nil).Once()
		mockAdmin.On("ListAuditByTarget", mock.Anything, "tpl1", 50).Return([]*models.AdminAuditLog{}, nil).Once()
		resp, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: note})
		assert.NoError(t, err)
		assert.Equal(t, "resolved_upheld", resp.State)
	})

	t.Run("SuccessReversed", func(t *testing.T) {
		svc, mockUser, mockAdmin, appealRepo, notif, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin2")
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		appealRepo.On("ResolveAppeal", mock.Anything, "a1", "reversed", note, "admin1").Return(nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Twice() // appeal.resolve + template.restore
		notif.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.Type == "appeal_resolved" && n.RecipientID == "alice"
		})).Return(nil).Once()
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(&models.AdminTemplate{Title: "T"}, []*models.AdminTemplateVersion{}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", DisplayName: "Alice"}, nil).Once()
		mockAdmin.On("ListAuditByTarget", mock.Anything, "tpl1", 50).Return([]*models.AdminAuditLog{}, nil).Once()
		resp, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "reversed", Note: note})
		assert.NoError(t, err)
		assert.Equal(t, "resolved_reversed", resp.State)
	})

	t.Run("NotifyExcerptTruncated", func(t *testing.T) {
		svc, mockUser, mockAdmin, appealRepo, notif, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin2")
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		longNote := "This is a very long resolution note that exceeds one hundred and forty characters in length so that the notification excerpt truncation logic kicks in and shortens it with an ellipsis character at the end for display purposes."
		appealRepo.On("ResolveAppeal", mock.Anything, "a1", "upheld", longNote, "admin1").Return(nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		notif.On("Create", mock.Anything, mock.MatchedBy(func(n *models.Notification) bool {
			return n.Type == "appeal_resolved" && len(n.ActorDisplayName) < 200
		})).Return(nil).Once()
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(&models.AdminTemplate{Title: "T"}, []*models.AdminTemplateVersion{}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", DisplayName: "Alice"}, nil).Once()
		mockAdmin.On("ListAuditByTarget", mock.Anything, "tpl1", 50).Return([]*models.AdminAuditLog{}, nil).Once()
		_, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: longNote})
		assert.NoError(t, err)
	})

	t.Run("StatementRenderedAsText", func(t *testing.T) {
		// Statements/notes are plain text; the proto carries them verbatim.
		svc, mockUser, mockAdmin, appealRepo, notif, _ := newAdminAppealService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		appeal, action := adminAppealDetail("a1", "open", "admin2")
		appeal.Statement = "<script>alert('xss')</script>" + statement
		appealRepo.On("GetAppeal", mock.Anything, "a1").Return(appeal, nil).Once()
		appealRepo.On("GetModerationAction", mock.Anything, "act1").Return(action, nil).Once()
		appealRepo.On("ResolveAppeal", mock.Anything, "a1", "upheld", note, "admin1").Return(nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(nil).Once()
		notif.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "tpl1").Return(&models.AdminTemplate{Title: "T"}, []*models.AdminTemplateVersion{}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "alice").Return(&models.User{ID: "alice", DisplayName: "Alice"}, nil).Once()
		mockAdmin.On("ListAuditByTarget", mock.Anything, "tpl1", 50).Return([]*models.AdminAuditLog{}, nil).Once()
		resp, err := svc.ResolveAppeal(adminCtx("admin1"), &pb.ResolveAppealRequest{AppealId: "a1", Outcome: "upheld", Note: note})
		assert.NoError(t, err)
		assert.Contains(t, resp.Statement, "<script>")
	})
}