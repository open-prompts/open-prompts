package service

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---- mocks ----

// MockAdminRepository is a mock implementation of repository.AdminRepository.
type MockAdminRepository struct {
	mock.Mock
}

func (m *MockAdminRepository) ListUsers(ctx context.Context, f repository.UserFilters) ([]*models.User, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.User), args.Get(1).(int64), args.Error(2)
}

func (m *MockAdminRepository) GetUserDetail(ctx context.Context, userID string) (*models.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockAdminRepository) CountUserTemplates(ctx context.Context, userID string) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockAdminRepository) GetUserStats(ctx context.Context, userID string) (*models.UserStats, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserStats), args.Error(1)
}

func (m *MockAdminRepository) ListUserIdentities(ctx context.Context, userID string) ([]*models.OAuthIdentity, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.OAuthIdentity), args.Error(1)
}

func (m *MockAdminRepository) ListTemplates(ctx context.Context, f repository.TemplateFilters) ([]*models.AdminTemplate, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.AdminTemplate), args.Get(1).(int64), args.Error(2)
}

func (m *MockAdminRepository) GetTemplateWithVersions(ctx context.Context, templateID string) (*models.AdminTemplate, []*models.AdminTemplateVersion, error) {
	args := m.Called(ctx, templateID)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	var versions []*models.AdminTemplateVersion
	if args.Get(1) != nil {
		versions = args.Get(1).([]*models.AdminTemplateVersion)
	}
	return args.Get(0).(*models.AdminTemplate), versions, args.Error(2)
}

func (m *MockAdminRepository) GetStats(ctx context.Context) (*models.AdminStats, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AdminStats), args.Error(1)
}

func (m *MockAdminRepository) ListRecentSignups(ctx context.Context, limit int) ([]*models.User, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.User), args.Error(1)
}

func (m *MockAdminRepository) ListRecentPublicTemplates(ctx context.Context, limit int) ([]*models.AdminTemplate, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.AdminTemplate), args.Error(1)
}

func (m *MockAdminRepository) InsertAuditLog(ctx context.Context, entry *models.AdminAuditLog) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

func (m *MockAdminRepository) ListAuditLogs(ctx context.Context, f repository.AuditFilters) ([]*models.AdminAuditLog, int64, error) {
	args := m.Called(ctx, f)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.AdminAuditLog), args.Get(1).(int64), args.Error(2)
}

func (m *MockAdminRepository) ListAuditByTarget(ctx context.Context, targetID string, limit int) ([]*models.AdminAuditLog, error) {
	args := m.Called(ctx, targetID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.AdminAuditLog), args.Error(1)
}

// MockAPIKeyLister is a mock implementation of service.APIKeyLister.
type MockAPIKeyLister struct {
	mock.Mock
}

func (m *MockAPIKeyLister) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.APIKey, error) {
	args := m.Called(ctx, userID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.APIKey), args.Error(1)
}

// ---- helpers ----

func newAdminTestService(t *testing.T) (*AdminService, *MockUserRepository, *MockAdminRepository, *MockRedisStore, *MockAPIKeyLister) {
	t.Helper()
	mockUser := new(MockUserRepository)
	mockAdmin := new(MockAdminRepository)
	mockRedis := new(MockRedisStore)
	mockKeys := new(MockAPIKeyLister)
	svc := NewAdminService(mockUser, mockAdmin, mockKeys, mockRedis, "secret", 30*time.Minute)
	return svc, mockUser, mockAdmin, mockRedis, mockKeys
}

func adminCtx(userID string) context.Context {
	ctx := ContextWithUserID(context.Background(), userID)
	ctx = ContextWithUserRole(ctx, "admin")
	ctx = ContextWithUserScope(ctx, "admin")
	return ctx
}

func makeAdminUser(id string) *models.User {
	return &models.User{
		ID:          id,
		Email:       id + "@example.com",
		DisplayName: "Admin " + id,
		Role:        "admin",
		Status:      "active",
	}
}

func makeRegularUser(id string) *models.User {
	return &models.User{
		ID:          id,
		Email:       id + "@example.com",
		DisplayName: "User " + id,
		Role:        "user",
		Status:      "active",
	}
}

// expectAdminAuth wires the requireAdminContext lookups for the actor.
func expectAdminAuth(mockUser *MockUserRepository, actor *models.User) {
	mockUser.On("GetByID", mock.Anything, actor.ID).Return(actor, nil).Maybe()
}

// ---- AdminLogin ----

func TestAdminLogin(t *testing.T) {
	password := "Admin_123456"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	admin := makeAdminUser("admin_1")
	admin.PasswordHash = string(hash)

	t.Run("Success", func(t *testing.T) {
		svc, mockUser, _, mockRedis, _ := newAdminTestService(t)
		mockUser.On("GetByEmail", mock.Anything, admin.Email).Return(admin, nil)
		mockRedis.On("Get", mock.Anything, "admin_login_attempts:unknown").Return("", errors.New("not found"))
		mockRedis.On("Del", mock.Anything, "admin_login_attempts:unknown").Return(nil)

		resp, err := svc.AdminLogin(context.Background(), &pb.AdminLoginRequest{Email: admin.Email, Password: password})
		assert.NoError(t, err)
		assert.NotEmpty(t, resp.Token)
		assert.Equal(t, admin.ID, resp.User.Id)
		assert.Equal(t, "admin", resp.User.Role)
	})

	t.Run("NonAdminRejected", func(t *testing.T) {
		svc, mockUser, _, mockRedis, _ := newAdminTestService(t)
		regular := makeRegularUser("user_1")
		regular.PasswordHash = string(hash)
		mockUser.On("GetByEmail", mock.Anything, regular.Email).Return(regular, nil)
		mockRedis.On("Get", mock.Anything, mock.Anything).Return("", errors.New("not found"))
		mockRedis.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		_, err := svc.AdminLogin(context.Background(), &pb.AdminLoginRequest{Email: regular.Email, Password: password})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("BannedAdminRejected", func(t *testing.T) {
		svc, mockUser, _, mockRedis, _ := newAdminTestService(t)
		banned := makeAdminUser("admin_2")
		banned.PasswordHash = string(hash)
		banned.Status = "banned"
		mockUser.On("GetByEmail", mock.Anything, banned.Email).Return(banned, nil)
		mockRedis.On("Get", mock.Anything, mock.Anything).Return("", errors.New("not found"))
		mockRedis.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		_, err := svc.AdminLogin(context.Background(), &pb.AdminLoginRequest{Email: banned.Email, Password: password})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("RateLimited", func(t *testing.T) {
		svc, _, _, mockRedis, _ := newAdminTestService(t)
		mockRedis.On("Get", mock.Anything, "admin_login_attempts:unknown").Return("5", nil)

		_, err := svc.AdminLogin(context.Background(), &pb.AdminLoginRequest{Email: "x@example.com", Password: "wrong"})
		assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	})

	t.Run("MissingFields", func(t *testing.T) {
		svc, _, _, _, _ := newAdminTestService(t)
		_, err := svc.AdminLogin(context.Background(), &pb.AdminLoginRequest{Email: "", Password: ""})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// ---- requireAdminContext ----

func TestRequireAdminContext(t *testing.T) {
	t.Run("MissingUserID", func(t *testing.T) {
		svc, _, _, _, _ := newAdminTestService(t)
		_, err := svc.GetAdminStats(context.Background(), &pb.GetAdminStatsRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("NonAdminRole", func(t *testing.T) {
		svc, _, _, _, _ := newAdminTestService(t)
		ctx := ContextWithUserID(context.Background(), "user_1")
		ctx = ContextWithUserRole(ctx, "user")
		ctx = ContextWithUserScope(ctx, "admin")
		_, err := svc.GetAdminStats(ctx, &pb.GetAdminStatsRequest{})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("MissingAdminScope", func(t *testing.T) {
		svc, _, _, _, _ := newAdminTestService(t)
		ctx := ContextWithUserID(context.Background(), "admin_1")
		ctx = ContextWithUserRole(ctx, "admin")
		_, err := svc.GetAdminStats(ctx, &pb.GetAdminStatsRequest{})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("DemotedAdminTokenRejected", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		demoted := makeRegularUser("admin_1") // token says admin, DB says user
		mockUser.On("GetByID", mock.Anything, "admin_1").Return(demoted, nil)
		_, err := svc.GetAdminStats(adminCtx("admin_1"), &pb.GetAdminStatsRequest{})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

// ---- GetAdminStats ----

func TestGetAdminStats(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		actor := makeAdminUser("admin_1")
		expectAdminAuth(mockUser, actor)
		mockAdmin.On("GetStats", mock.Anything).Return(&models.AdminStats{
			TotalUsers: 10, ActiveUsers: 9, BannedUsers: 1,
			TotalTemplates: 20, PublicTemplates: 15, PromptsCreated: 30,
		}, nil)
		mockAdmin.On("ListRecentSignups", mock.Anything, 10).Return([]*models.User{makeRegularUser("u1")}, nil)
		mockAdmin.On("ListRecentPublicTemplates", mock.Anything, 10).Return([]*models.AdminTemplate{{ID: "t1", Title: "T"}}, nil)

		resp, err := svc.GetAdminStats(adminCtx("admin_1"), &pb.GetAdminStatsRequest{})
		assert.NoError(t, err)
		assert.Equal(t, int64(10), resp.TotalUsers)
		assert.Equal(t, int64(1), resp.BannedUsers)
		assert.Len(t, resp.RecentSignups, 1)
		assert.Len(t, resp.RecentTemplates, 1)
	})
}

// ---- ListUsers ----

func TestListUsers(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		users := []*models.User{makeRegularUser("u1"), makeRegularUser("u2")}
		mockAdmin.On("ListUsers", mock.Anything, repository.UserFilters{
			Search: "", Status: "", Role: "", Sort: "", Limit: 20, Offset: 0,
		}).Return(users, int64(2), nil)
		mockAdmin.On("CountUserTemplates", mock.Anything, "u1").Return(int64(3), nil)
		mockAdmin.On("CountUserTemplates", mock.Anything, "u2").Return(int64(0), nil)

		resp, err := svc.ListUsers(adminCtx("admin_1"), &pb.ListUsersRequest{})
		assert.NoError(t, err)
		assert.Equal(t, int64(2), resp.Total)
		assert.Len(t, resp.Users, 2)
		assert.Equal(t, int32(3), resp.Users[0].TemplateCount)
	})

	t.Run("InvalidStatusFilter", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ListUsers(adminCtx("admin_1"), &pb.ListUsersRequest{Status: "weird"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("InvalidRoleFilter", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ListUsers(adminCtx("admin_1"), &pb.ListUsersRequest{Role: "superuser"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("InvalidSort", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ListUsers(adminCtx("admin_1"), &pb.ListUsersRequest{Sort: "random"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("PerPageClamped", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("ListUsers", mock.Anything, repository.UserFilters{Limit: 100, Offset: 0}).Return([]*models.User{}, int64(0), nil)

		resp, err := svc.ListUsers(adminCtx("admin_1"), &pb.ListUsersRequest{PerPage: 500})
		assert.NoError(t, err)
		assert.Equal(t, int32(100), resp.PerPage)
	})
}

// ---- GetUser ----

func TestGetUser(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, mockKeys := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		target := makeRegularUser("u1")
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(target, nil)
		mockAdmin.On("GetUserStats", mock.Anything, "u1").Return(&models.UserStats{Templates: 5, LikesReceived: 10}, nil)
		mockAdmin.On("ListUserIdentities", mock.Anything, "u1").Return([]*models.OAuthIdentity{{Provider: "github"}}, nil)
		mockKeys.On("ListByUserID", mock.Anything, "u1", 100, 0).Return([]*models.APIKey{}, nil)
		mockAdmin.On("ListTemplates", mock.Anything, repository.TemplateFilters{OwnerID: "u1", Sort: "recently_updated", Limit: 5, Offset: 0}).Return([]*models.AdminTemplate{}, int64(5), nil)
		mockAdmin.On("ListAuditByTarget", mock.Anything, "u1", 20).Return([]*models.AdminAuditLog{}, nil)

		resp, err := svc.GetUser(adminCtx("admin_1"), &pb.GetUserRequest{Id: "u1"})
		assert.NoError(t, err)
		assert.Equal(t, "u1", resp.User.Id)
		assert.Equal(t, int64(5), resp.Stats.Templates)
		assert.Equal(t, int32(5), resp.User.TemplateCount)
		assert.Len(t, resp.Identities, 1)
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("GetUserDetail", mock.Anything, "missing").Return(nil, repository.ErrUserNotFound)

		_, err := svc.GetUser(adminCtx("admin_1"), &pb.GetUserRequest{Id: "missing"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("EmptyID", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.GetUser(adminCtx("admin_1"), &pb.GetUserRequest{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// ---- BanUser ----

func TestBanUser(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, mockRedis, _ := newAdminTestService(t)
		actor := makeAdminUser("admin_1")
		expectAdminAuth(mockUser, actor)
		target := makeRegularUser("u1")
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(target, nil)
		mockUser.On("UpdateRoleAndStatus", mock.Anything, "u1", "user", "banned").Return(nil)
		mockRedis.On("Set", mock.Anything, "user_banned:u1", "1", mock.Anything).Return(nil)
		mockRedis.On("Set", mock.Anything, "token_revoked_before:u1", mock.Anything, mock.Anything).Return(nil)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.MatchedBy(func(e *models.AdminAuditLog) bool {
			return e.Action == "ban" && e.TargetID == "u1" && e.ActorID == actor.ID && e.Reason == "spamming the platform"
		})).Return(nil)

		resp, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: "u1", Reason: "spamming the platform"})
		assert.NoError(t, err)
		assert.Equal(t, "banned", resp.User.Status)
	})

	t.Run("SelfBanRejected", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: "admin_1", Reason: "self ban attempt"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("AlreadyBanned", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		banned := makeRegularUser("u1")
		banned.Status = "banned"
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(banned, nil)

		_, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: "u1", Reason: "ban again please"})
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("ShortReasonRejected", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: "u1", Reason: "no"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("LongReasonRejected", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: "u1", Reason: string(make([]byte, 501))})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("GetUserDetail", mock.Anything, "missing").Return(nil, repository.ErrUserNotFound)

		_, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: "missing", Reason: "does not exist"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("AuditFailureNotFatal", func(t *testing.T) {
		svc, mockUser, mockAdmin, mockRedis, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(makeRegularUser("u1"), nil)
		mockUser.On("UpdateRoleAndStatus", mock.Anything, "u1", "user", "banned").Return(nil)
		mockRedis.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.Anything).Return(errors.New("audit write failed"))

		resp, err := svc.BanUser(adminCtx("admin_1"), &pb.BanUserRequest{UserId: "u1", Reason: "spamming the platform"})
		assert.NoError(t, err)
		assert.Equal(t, "banned", resp.User.Status)
	})
}

// ---- UnbanUser ----

func TestUnbanUser(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, mockRedis, _ := newAdminTestService(t)
		actor := makeAdminUser("admin_1")
		expectAdminAuth(mockUser, actor)
		banned := makeRegularUser("u1")
		banned.Status = "banned"
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(banned, nil)
		mockUser.On("UpdateRoleAndStatus", mock.Anything, "u1", "user", "active").Return(nil)
		mockRedis.On("Del", mock.Anything, "user_banned:u1").Return(nil)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.MatchedBy(func(e *models.AdminAuditLog) bool {
			return e.Action == "unban" && e.TargetID == "u1"
		})).Return(nil)

		resp, err := svc.UnbanUser(adminCtx("admin_1"), &pb.UnbanUserRequest{UserId: "u1", Reason: "appeal accepted"})
		assert.NoError(t, err)
		assert.Equal(t, "active", resp.User.Status)
	})

	t.Run("NotBanned", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(makeRegularUser("u1"), nil)

		_, err := svc.UnbanUser(adminCtx("admin_1"), &pb.UnbanUserRequest{UserId: "u1", Reason: "appeal accepted"})
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})
}

// ---- ChangeUserRole ----

func TestChangeUserRole(t *testing.T) {
	t.Run("PromoteSuccess", func(t *testing.T) {
		svc, mockUser, mockAdmin, mockRedis, _ := newAdminTestService(t)
		actor := makeAdminUser("admin_1")
		expectAdminAuth(mockUser, actor)
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(makeRegularUser("u1"), nil)
		mockUser.On("UpdateRoleAndStatus", mock.Anything, "u1", "admin", "active").Return(nil)
		mockRedis.On("Set", mock.Anything, "token_revoked_before:u1", mock.Anything, mock.Anything).Return(nil)
		mockAdmin.On("InsertAuditLog", mock.Anything, mock.MatchedBy(func(e *models.AdminAuditLog) bool {
			return e.Action == "role_change" && e.TargetID == "u1"
		})).Return(nil)

		resp, err := svc.ChangeUserRole(adminCtx("admin_1"), &pb.ChangeUserRoleRequest{UserId: "u1", Role: "admin", Reason: "joining the team"})
		assert.NoError(t, err)
		assert.Equal(t, "admin", resp.User.Role)
	})

	t.Run("SelfDemotionRejected", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ChangeUserRole(adminCtx("admin_1"), &pb.ChangeUserRoleRequest{UserId: "admin_1", Role: "user", Reason: "stepping down now"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("SameRoleConflict", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("GetUserDetail", mock.Anything, "u1").Return(makeRegularUser("u1"), nil)

		_, err := svc.ChangeUserRole(adminCtx("admin_1"), &pb.ChangeUserRoleRequest{UserId: "u1", Role: "user", Reason: "no change here"})
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	})

	t.Run("InvalidRole", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ChangeUserRole(adminCtx("admin_1"), &pb.ChangeUserRoleRequest{UserId: "u1", Role: "superadmin", Reason: "invalid role value"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// ---- ListTemplates / GetTemplate ----

func TestListTemplates(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("ListTemplates", mock.Anything, repository.TemplateFilters{Limit: 20, Offset: 0}).Return([]*models.AdminTemplate{
			{ID: "t1", Title: "Private one", Visibility: "private"},
		}, int64(1), nil)

		resp, err := svc.ListTemplates(adminCtx("admin_1"), &pb.ListAdminTemplatesRequest{})
		assert.NoError(t, err)
		assert.Equal(t, int64(1), resp.Total)
		assert.Len(t, resp.Templates, 1)
		assert.Equal(t, "private", resp.Templates[0].Visibility)
	})

	t.Run("InvalidVisibility", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ListTemplates(adminCtx("admin_1"), &pb.ListAdminTemplatesRequest{Visibility: "hidden"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("InvalidSort", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ListTemplates(adminCtx("admin_1"), &pb.ListAdminTemplatesRequest{Sort: "random"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestGetTemplate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		tmpl := &models.AdminTemplate{ID: "t1", Title: "T", Visibility: "public"}
		versions := []*models.AdminTemplateVersion{{ID: 1, Version: 2, Content: "latest"}, {ID: 2, Version: 1, Content: "first"}}
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "t1").Return(tmpl, versions, nil)

		resp, err := svc.GetTemplate(adminCtx("admin_1"), &pb.GetAdminTemplateRequest{Id: "t1"})
		assert.NoError(t, err)
		assert.Equal(t, "t1", resp.Template.Id)
		assert.Len(t, resp.Versions, 2)
		assert.Equal(t, int32(2), resp.Versions[0].Version)
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("GetTemplateWithVersions", mock.Anything, "missing").Return(nil, nil, repository.ErrTemplateNotFound)

		_, err := svc.GetTemplate(adminCtx("admin_1"), &pb.GetAdminTemplateRequest{Id: "missing"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

// ---- ListAuditLogs ----

func TestListAuditLogs(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, mockAdmin, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		mockAdmin.On("ListAuditLogs", mock.Anything, repository.AuditFilters{Limit: 20, Offset: 0}).Return([]*models.AdminAuditLog{
			{ID: "a1", Action: "ban", TargetID: "u1", Reason: "spam"},
		}, int64(1), nil)

		resp, err := svc.ListAuditLogs(adminCtx("admin_1"), &pb.ListAuditLogsRequest{})
		assert.NoError(t, err)
		assert.Equal(t, int64(1), resp.Total)
		assert.Len(t, resp.Entries, 1)
		assert.Equal(t, "ban", resp.Entries[0].Action)
	})

	t.Run("InvalidAction", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))
		_, err := svc.ListAuditLogs(adminCtx("admin_1"), &pb.ListAuditLogsRequest{Action: "delete"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// ---- RefreshAdminSession ----

func TestRefreshAdminSession(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		svc, mockUser, _, _, _ := newAdminTestService(t)
		expectAdminAuth(mockUser, makeAdminUser("admin_1"))

		resp, err := svc.RefreshAdminSession(adminCtx("admin_1"), &pb.RefreshAdminSessionRequest{})
		assert.NoError(t, err)
		assert.NotEmpty(t, resp.Token)
	})

	t.Run("RequiresAdmin", func(t *testing.T) {
		svc, _, _, _, _ := newAdminTestService(t)
		_, err := svc.RefreshAdminSession(context.Background(), &pb.RefreshAdminSessionRequest{})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

// ---- BootstrapAdmin ----

func TestBootstrapAdmin(t *testing.T) {
	t.Run("DisabledWhenEmailEmpty", func(t *testing.T) {
		mockUser := new(MockUserRepository)
		err := BootstrapAdmin(context.Background(), mockUser, "", "password")
		assert.NoError(t, err)
		mockUser.AssertNotCalled(t, "GetByEmail")
	})

	t.Run("ShortPasswordRejected", func(t *testing.T) {
		mockUser := new(MockUserRepository)
		err := BootstrapAdmin(context.Background(), mockUser, "admin@example.com", "short")
		assert.Error(t, err)
	})

	t.Run("PromotesExistingUser", func(t *testing.T) {
		mockUser := new(MockUserRepository)
		existing := makeRegularUser("u1")
		existing.Email = "admin@example.com"
		mockUser.On("GetByEmail", mock.Anything, "admin@example.com").Return(existing, nil)
		mockUser.On("UpdateRoleAndStatus", mock.Anything, "u1", "admin", "active").Return(nil)

		err := BootstrapAdmin(context.Background(), mockUser, "admin@example.com", "Admin_123456")
		assert.NoError(t, err)
	})

	t.Run("CreatesNewAdmin", func(t *testing.T) {
		mockUser := new(MockUserRepository)
		mockUser.On("GetByEmail", mock.Anything, "admin@example.com").Return(nil, repository.ErrUserNotFound)
		mockUser.On("Insert", mock.Anything, mock.Anything).Return(nil)

		err := BootstrapAdmin(context.Background(), mockUser, "admin@example.com", "Admin_123456")
		assert.NoError(t, err)
	})
}

// ---- validateReason ----

func TestValidateReason(t *testing.T) {
	assert.Error(t, validateReason(""))
	assert.Error(t, validateReason("  ab  "))
	assert.NoError(t, validateReason("valid reason"))
	assert.NoError(t, validateReason(string(make([]byte, 500))))
	assert.Error(t, validateReason(string(make([]byte, 501))))
}
