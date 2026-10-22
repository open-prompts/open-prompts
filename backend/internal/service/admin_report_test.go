package service

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errTest = errors.New("test error")

func newAdminReportService(t *testing.T) (*AdminService, *MockUserRepository, *MockAdminRepository, *MockReportRepository, *MockCommentRepository) {
	t.Helper()
	svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
	reportRepo := new(MockReportRepository)
	svc.SetReportRepo(reportRepo)
	commentRepo := new(MockCommentRepository)
	svc.SetCommentRepo(commentRepo)
	return svc, mockUser, mockAdmin, reportRepo, commentRepo
}

func TestListReportQueue(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _ := newAdminReportService(t)
		_, err := svc.ListReportQueue(context.Background(), &pb.ListReportQueueRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("InvalidStatus", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		_, err := svc.ListReportQueue(adminCtx("admin1"), &pb.ListReportQueueRequest{Status: "bogus"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc, mockUser, _, reportRepo, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		reportRepo.On("ListQueue", mock.Anything, "open", "", "", 50, 0).Return([]*models.ReportGroup{
			{ItemType: "template", ItemID: "t1", ItemTitle: "T", AuthorDisplayName: "Alice", OpenCount: 3, Reasons: []string{"spam"}, LatestAt: time.Now(), Status: "open"},
		}, int64(1), nil).Once()
		resp, err := svc.ListReportQueue(adminCtx("admin1"), &pb.ListReportQueueRequest{Status: "open"})
		assert.NoError(t, err)
		assert.Len(t, resp.Groups, 1)
		assert.Equal(t, int32(3), resp.Groups[0].OpenCount)
		assert.Equal(t, int64(1), resp.Total)
	})
}

func TestGetReportDetail(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _ := newAdminReportService(t)
		_, err := svc.GetReportDetail(context.Background(), &pb.GetReportDetailRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NoReports", func(t *testing.T) {
		svc, mockUser, _, reportRepo, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		reportRepo.On("ListForItem", mock.Anything, "template", "t1").Return([]*models.ContentReport{}, nil).Once()
		_, err := svc.GetReportDetail(adminCtx("admin1"), &pb.GetReportDetailRequest{ItemType: "template", ItemId: "t1"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("SuccessTemplate", func(t *testing.T) {
		svc, mockUser, mockAdmin, reportRepo, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		reportRepo.On("ListForItem", mock.Anything, "template", "t1").Return([]*models.ContentReport{
			{ID: "r1", ReporterID: "u2", ReporterDisplayName: "Eve", Reason: "spam", Note: "note", Status: "open", CreatedAt: time.Now()},
		}, nil).Once()
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "t1").Return(&models.AdminTemplate{Title: "My Tpl", OwnerID: "u2"}, []*models.AdminTemplateVersion{}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "u2").Return(&models.User{ID: "u2", DisplayName: "Eve"}, nil).Once()
		resp, err := svc.GetReportDetail(adminCtx("admin1"), &pb.GetReportDetailRequest{ItemType: "template", ItemId: "t1"})
		assert.NoError(t, err)
		assert.Equal(t, "My Tpl", resp.ItemTitle)
		assert.Equal(t, "Eve", resp.AuthorDisplayName)
		assert.Len(t, resp.Reports, 1)
		assert.Equal(t, "Eve", resp.Reports[0].ReporterDisplayName)
	})

	t.Run("SuccessComment", func(t *testing.T) {
		svc, mockUser, _, reportRepo, commentRepo := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		reportRepo.On("ListForItem", mock.Anything, "comment", "c1").Return([]*models.ContentReport{
			{ID: "r1", ReporterID: "u2", Reason: "offensive", Status: "open", CreatedAt: time.Now()},
		}, nil).Once()
		commentRepo.On("Get", mock.Anything, "c1").Return(&models.Comment{ID: "c1", UserID: "u2", Content: "bad comment"}, nil).Once()
		mockUser.On("GetByID", mock.Anything, "u2").Return(&models.User{ID: "u2", DisplayName: "Eve"}, nil).Once()
		resp, err := svc.GetReportDetail(adminCtx("admin1"), &pb.GetReportDetailRequest{ItemType: "comment", ItemId: "c1"})
		assert.NoError(t, err)
		assert.Equal(t, "bad comment", resp.ItemContent)
		assert.Equal(t, "Eve", resp.AuthorDisplayName)
	})
}

func TestResolveReportGroup(t *testing.T) {
	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _ := newAdminReportService(t)
		_, err := svc.ResolveReportGroup(context.Background(), &pb.ResolveReportGroupRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("InvalidResolution", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		_, err := svc.ResolveReportGroup(adminCtx("admin1"), &pb.ResolveReportGroupRequest{
			ItemType: "template", ItemId: "t1", Resolution: "bogus", Reason: "a valid reason",
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("ReasonTooShort", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		_, err := svc.ResolveReportGroup(adminCtx("admin1"), &pb.ResolveReportGroupRequest{
			ItemType: "template", ItemId: "t1", Resolution: "dismissed", Reason: "ab",
		})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("NoOpenReports", func(t *testing.T) {
		svc, mockUser, _, reportRepo, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		reportRepo.On("ResolveItem", mock.Anything, "template", "t1", "dismissed", "admin1").Return(int64(0), nil).Once()
		_, err := svc.ResolveReportGroup(adminCtx("admin1"), &pb.ResolveReportGroupRequest{
			ItemType: "template", ItemId: "t1", Resolution: "dismissed", Reason: "a valid reason",
		})
		assert.Equal(t, codes.AlreadyExists, status.Code(err))
	})

	t.Run("SuccessDismissed", func(t *testing.T) {
		svc, mockUser, mockAdmin, reportRepo, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		reportRepo.On("ResolveItem", mock.Anything, "template", "t1", "dismissed", "admin1").Return(int64(2), nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.MatchedBy(func(e *models.AdminAuditLog) bool {
			return e.Action == "report.dismiss" && e.TargetType == "template" && e.TargetID == "t1"
		})).Return(nil).Once()
		resp, err := svc.ResolveReportGroup(adminCtx("admin1"), &pb.ResolveReportGroupRequest{
			ItemType: "template", ItemId: "t1", Resolution: "dismissed", Reason: "a valid reason",
		})
		assert.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, int32(2), resp.ResolvedCount)
	})

	t.Run("SuccessActioned", func(t *testing.T) {
		svc, mockUser, mockAdmin, reportRepo, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		reportRepo.On("ResolveItem", mock.Anything, "comment", "c1", "actioned", "admin1").Return(int64(1), nil).Once()
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.MatchedBy(func(e *models.AdminAuditLog) bool {
			return e.Action == "report.actioned" && e.TargetType == "comment" && e.TargetID == "c1"
		})).Return(nil).Once()
		resp, err := svc.ResolveReportGroup(adminCtx("admin1"), &pb.ResolveReportGroupRequest{
			ItemType: "comment", ItemId: "c1", Resolution: "actioned", Reason: "a valid reason",
		})
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	})
}

func TestGetAdminStatsOpenReports(t *testing.T) {
	t.Run("IncludesOpenReports", func(t *testing.T) {
		svc, mockUser, mockAdmin, reportRepo, _ := newAdminReportService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin1"))
		mockAdmin.On("GetStats", mock.Anything).Return(&models.AdminStats{TotalUsers: 10}, nil).Once()
		mockAdmin.On("ListRecentSignups", mock.Anything, 10).Return([]*models.User{}, nil).Once()
		mockAdmin.On("ListRecentPublicTemplates", mock.Anything, 10).Return([]*models.AdminTemplate{}, nil).Once()
		reportRepo.On("CountOpen", mock.Anything).Return(int64(7), nil).Once()
		resp, err := svc.GetAdminStats(adminCtx("admin1"), &pb.GetAdminStatsRequest{})
		assert.NoError(t, err)
		assert.Equal(t, int64(7), resp.OpenReports)
	})
}

func TestAutoResolveItemHook(t *testing.T) {
	t.Run("CallsAutoResolve", func(t *testing.T) {
		svc, _, _, reportRepo, _ := newAdminReportService(t)
		reportRepo.On("AutoResolveItem", mock.Anything, "comment", "c1", "admin1").Return(nil).Once()
		svc.autoResolveItem(context.Background(), "comment", "c1", "admin1")
		reportRepo.AssertExpectations(t)
	})

	t.Run("NilRepoNoop", func(t *testing.T) {
		svc, _, _, _, _ := newAdminReportService(t)
		svc.ReportRepo = nil
		svc.autoResolveItem(context.Background(), "comment", "c1", "admin1") // must not panic
	})

	t.Run("ErrorLogged", func(t *testing.T) {
		svc, _, _, reportRepo, _ := newAdminReportService(t)
		reportRepo.On("AutoResolveItem", mock.Anything, "comment", "c1", "admin1").Return(errTest).Once()
		svc.autoResolveItem(context.Background(), "comment", "c1", "admin1") // must not panic
	})
}