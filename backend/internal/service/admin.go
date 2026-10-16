package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminLoginMaxAttempts  = 5
	adminLoginWindow       = 15 * time.Minute
	adminBanKeyTTL         = 25 * time.Hour
	defaultAdminSessionTTL = 30 * time.Minute
)

// APIKeyLister is the subset of APIKeyRepository used by the admin service.
type APIKeyLister interface {
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.APIKey, error)
}

// AdminService implements the admin console RPCs.
type AdminService struct {
	pb.UnimplementedAdminServiceServer

	UserRepo         repository.UserRepository
	AdminRepo        repository.AdminRepository
	APIKeyRepo       APIKeyLister
	ReviewRepo       repository.ReviewRepository
	NotificationRepo repository.NotificationRepository
	Redis            RedisStore
	JWTSecret        []byte
	SessionTTL       time.Duration

	// promptSvc is wired post-construction so UpdatePlatformSetting can
	// invalidate the prompt service's settings cache.
	promptSvc *PromptService
}

// NewAdminService creates a new AdminService.
func NewAdminService(
	userRepo repository.UserRepository,
	adminRepo repository.AdminRepository,
	apiKeyRepo APIKeyLister,
	redis RedisStore,
	jwtSecret string,
	sessionTTL time.Duration,
) *AdminService {
	if sessionTTL <= 0 {
		sessionTTL = defaultAdminSessionTTL
	}
	return &AdminService{
		UserRepo:   userRepo,
		AdminRepo:  adminRepo,
		APIKeyRepo: apiKeyRepo,
		Redis:      redis,
		JWTSecret:  []byte(jwtSecret),
		SessionTTL: sessionTTL,
	}
}

// SetReviewRepo wires the review repository (post-construction so existing
// call sites stay unchanged).
func (s *AdminService) SetReviewRepo(repo repository.ReviewRepository) {
	s.ReviewRepo = repo
}

// SetPromptService wires the prompt service for settings-cache
// invalidation (post-construction).
func (s *AdminService) SetPromptService(ps *PromptService) {
	s.promptSvc = ps
}

// SetNotificationRepo wires the notification repository (post-construction)
// used by review-decision notifications.
func (s *AdminService) SetNotificationRepo(repo repository.NotificationRepository) {
	s.NotificationRepo = repo
}

// ---- helpers ----

// requireAdminContext verifies the caller holds an admin-scoped token.
func (s *AdminService) requireAdminContext(ctx context.Context) (*models.User, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if GetUserRoleFromContext(ctx) != "admin" || GetUserScopeFromContext(ctx) != "admin" {
		return nil, status.Error(codes.PermissionDenied, "admin privileges required")
	}
	user, err := s.UserRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	if user.Role != "admin" || user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "admin privileges required")
	}
	return user, nil
}

// generateAdminToken issues a short-lived admin-scoped JWT.
func (s *AdminService) generateAdminToken(user *models.User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"role":  "admin",
		"scope": "admin",
		"iat":   now.Unix(),
		"exp":   now.Add(s.SessionTTL).Unix(),
		"iss":   "open-prompts",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.JWTSecret)
}

// validateReason enforces the mandatory 3-500 char reason on admin mutations.
func validateReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if len(trimmed) < 3 || len(trimmed) > 500 {
		return status.Error(codes.InvalidArgument, "reason must be between 3 and 500 characters")
	}
	return nil
}

// writeAudit persists an audit entry; failures are logged, never fatal.
func (s *AdminService) writeAudit(ctx context.Context, actor *models.User, action, targetType, targetID, reason string) {
	entry := &models.AdminAuditLog{
		ActorID:    actor.ID,
		ActorEmail: actor.Email,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Reason:     strings.TrimSpace(reason),
	}
	if err := s.AdminRepo.InsertAuditLog(ctx, entry); err != nil {
		zap.S().Errorf("AdminService: failed to write audit log action=%s target=%s: %v", action, targetID, err)
	}
}

// taxonomy returns the config-driven warning keys (from platform_settings).
// Falls back to the built-in default on cache miss or invalid JSON.
func (s *AdminService) taxonomy(ctx context.Context) []string {
	if s.ReviewRepo == nil {
		return defaultWarningTaxonomy
	}
	value, err := s.ReviewRepo.GetSetting(ctx, settingWarningTaxonomy)
	if err != nil || value == "" {
		return defaultWarningTaxonomy
	}
	var keys []string
	if err := json.Unmarshal([]byte(value), &keys); err != nil {
		return defaultWarningTaxonomy
	}
	if len(keys) == 0 {
		return defaultWarningTaxonomy
	}
	return keys
}

// userToSummary converts a models.User to the proto summary.
func userToSummary(user *models.User, templateCount int32) *pb.AdminUserSummary {
	return &pb.AdminUserSummary{
		Id:            user.ID,
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		Avatar:        user.Avatar,
		Role:          user.Role,
		Status:        user.Status,
		TemplateCount: templateCount,
		CreatedAt:     timestamppb.New(user.CreatedAt),
		UpdatedAt:     timestamppb.New(user.UpdatedAt),
	}
}

// templateToSummary converts a models.AdminTemplate to the proto summary.
func templateToSummary(t *models.AdminTemplate) *pb.AdminTemplateSummary {
	ratingAvg := 0.0
	if t.RatingCount > 0 {
		ratingAvg = float64(t.RatingSum) / float64(t.RatingCount)
	}
	return &pb.AdminTemplateSummary{
		Id:               t.ID,
		Title:            t.Title,
		Visibility:       t.Visibility,
		Type:             t.Type,
		Language:         t.Language,
		Tags:             t.Tags,
		LikeCount:        t.LikeCount,
		FavoriteCount:    t.FavoriteCount,
		RatingAvg:        ratingAvg,
		RatingCount:      t.RatingCount,
		CreatedAt:        timestamppb.New(t.CreatedAt),
		UpdatedAt:        timestamppb.New(t.UpdatedAt),
		OwnerId:          t.OwnerID,
		OwnerDisplayName: t.OwnerDisplayName,
		Category:         t.Category,
		ContentWarnings:  t.ContentWarnings,
		LockedWarnings:   t.LockedWarnings,
	}
}

// clientIPFromContext extracts the client IP from the x-client-ip metadata
// (set by the REST gateway) or falls back to "unknown".
func clientIPFromContext(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("x-client-ip"); len(vals) > 0 && vals[0] != "" {
			return vals[0]
		}
	}
	return "unknown"
}

// ---- auth ----

// AdminLogin authenticates an admin with rate limiting (5 fails / 15 min / IP).
func (s *AdminService) AdminLogin(ctx context.Context, req *pb.AdminLoginRequest) (*pb.AdminLoginResponse, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	ip := clientIPFromContext(ctx)
	attemptKey := "admin_login_attempts:" + ip
	if s.Redis != nil {
		if v, err := s.Redis.Get(ctx, attemptKey); err == nil && v != "" {
			var attempts int
			if _, err := fmt.Sscanf(v, "%d", &attempts); err == nil && attempts >= adminLoginMaxAttempts {
				return nil, status.Error(codes.ResourceExhausted, "too many failed login attempts, try again later")
			}
		}
	}

	user, err := s.UserRepo.GetByEmail(ctx, email)
	if err != nil {
		s.recordFailedAdminLogin(ctx, attemptKey)
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		s.recordFailedAdminLogin(ctx, attemptKey)
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	if user.Role != "admin" {
		s.recordFailedAdminLogin(ctx, attemptKey)
		return nil, status.Error(codes.PermissionDenied, "admin privileges required")
	}
	if user.Status == "banned" {
		s.recordFailedAdminLogin(ctx, attemptKey)
		return nil, status.Error(codes.PermissionDenied, "account banned")
	}

	// Successful login clears the failure counter.
	if s.Redis != nil {
		_ = s.Redis.Del(ctx, attemptKey)
	}

	token, err := s.generateAdminToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}

	return &pb.AdminLoginResponse{
		Token: token,
		User:  userToSummary(user, 0),
	}, nil
}

// recordFailedAdminLogin increments the per-IP failure counter.
func (s *AdminService) recordFailedAdminLogin(ctx context.Context, key string) {
	if s.Redis == nil {
		return
	}
	v, err := s.Redis.Get(ctx, key)
	if err != nil || v == "" {
		_ = s.Redis.Set(ctx, key, "1", adminLoginWindow)
		return
	}
	var attempts int
	if _, err := fmt.Sscanf(v, "%d", &attempts); err != nil {
		_ = s.Redis.Set(ctx, key, "1", adminLoginWindow)
		return
	}
	_ = s.Redis.Set(ctx, key, fmt.Sprintf("%d", attempts+1), adminLoginWindow)
}

// RefreshAdminSession renews the admin token (sliding expiration).
func (s *AdminService) RefreshAdminSession(ctx context.Context, _ *pb.RefreshAdminSessionRequest) (*pb.RefreshAdminSessionResponse, error) {
	user, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.generateAdminToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}
	return &pb.RefreshAdminSessionResponse{Token: token}, nil
}

// ---- stats ----

// GetAdminStats returns dashboard KPIs and recent activity.
func (s *AdminService) GetAdminStats(ctx context.Context, _ *pb.GetAdminStatsRequest) (*pb.GetAdminStatsResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}

	stats, err := s.AdminRepo.GetStats(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get stats: %v", err)
	}

	recentSignups, err := s.AdminRepo.ListRecentSignups(ctx, 10)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get recent signups: %v", err)
	}
	recentTemplates, err := s.AdminRepo.ListRecentPublicTemplates(ctx, 10)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get recent templates: %v", err)
	}

	resp := &pb.GetAdminStatsResponse{
		TotalUsers:      stats.TotalUsers,
		ActiveUsers:     stats.ActiveUsers,
		BannedUsers:     stats.BannedUsers,
		TotalTemplates:  stats.TotalTemplates,
		PublicTemplates: stats.PublicTemplates,
		PromptsCreated:  stats.PromptsCreated,
	}
	for _, u := range recentSignups {
		resp.RecentSignups = append(resp.RecentSignups, userToSummary(u, 0))
	}
	for _, t := range recentTemplates {
		resp.RecentTemplates = append(resp.RecentTemplates, templateToSummary(t))
	}
	return resp, nil
}

// ---- users ----

// ListUsers returns a paginated, searchable, filterable user list.
func (s *AdminService) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}

	if req.Status != "" && req.Status != "active" && req.Status != "banned" {
		return nil, status.Error(codes.InvalidArgument, "invalid status filter")
	}
	if req.Role != "" && req.Role != "user" && req.Role != "admin" {
		return nil, status.Error(codes.InvalidArgument, "invalid role filter")
	}
	switch req.Sort {
	case "", "newest", "oldest", "recently_updated":
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid sort")
	}

	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	perPage := int(req.PerPage)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	users, total, err := s.AdminRepo.ListUsers(ctx, repository.UserFilters{
		Search: strings.TrimSpace(req.Search),
		Status: req.Status,
		Role:   req.Role,
		Sort:   req.Sort,
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
	}

	resp := &pb.ListUsersResponse{Total: total, Page: int32(page), PerPage: int32(perPage)}
	for _, u := range users {
		count, err := s.AdminRepo.CountUserTemplates(ctx, u.ID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to count templates: %v", err)
		}
		resp.Users = append(resp.Users, userToSummary(u, int32(count)))
	}
	return resp, nil
}

// GetUser returns full admin detail for one user.
func (s *AdminService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	user, err := s.AdminRepo.GetUserDetail(ctx, req.Id)
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}

	stats, err := s.AdminRepo.GetUserStats(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user stats: %v", err)
	}
	identities, err := s.AdminRepo.ListUserIdentities(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user identities: %v", err)
	}
	apiKeys, err := s.APIKeyRepo.ListByUserID(ctx, user.ID, 100, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user api keys: %v", err)
	}
	templates, total, err := s.AdminRepo.ListTemplates(ctx, repository.TemplateFilters{OwnerID: user.ID, Sort: "recently_updated", Limit: 5, Offset: 0})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user templates: %v", err)
	}
	history, err := s.AdminRepo.ListAuditByTarget(ctx, user.ID, 20)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get action history: %v", err)
	}

	resp := &pb.GetUserResponse{
		User: userToSummary(user, int32(total)),
		Stats: &pb.UserStats{
			Templates:         stats.Templates,
			LikesReceived:     stats.LikesReceived,
			FavoritesReceived: stats.FavoritesReceived,
			PromptsCreated:    stats.PromptsCreated,
			FollowerCount:     stats.FollowerCount,
			FollowingCount:    stats.FollowingCount,
		},
	}
	for _, id := range identities {
		resp.Identities = append(resp.Identities, &pb.OAuthIdentity{
			Provider:  id.Provider,
			CreatedAt: timestamppb.New(id.CreatedAt),
		})
	}
	for _, k := range apiKeys {
		key := &pb.AdminAPIKeySummary{
			Id:        k.ID,
			Name:      k.Name,
			Prefix:    k.Prefix,
			IsActive:  k.IsActive,
			CreatedAt: timestamppb.New(k.CreatedAt),
		}
		if k.LastUsedAt.Valid {
			key.LastUsedAt = timestamppb.New(k.LastUsedAt.Time)
		}
		resp.ApiKeys = append(resp.ApiKeys, key)
	}
	for _, t := range templates {
		resp.Templates = append(resp.Templates, templateToSummary(t))
	}
	for _, e := range history {
		resp.ActionHistory = append(resp.ActionHistory, auditLogToProto(e))
	}
	return resp, nil
}

// BanUser bans a user, invalidating live sessions and API keys.
func (s *AdminService) BanUser(ctx context.Context, req *pb.BanUserRequest) (*pb.UserActionResponse, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateReason(req.Reason); err != nil {
		return nil, err
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}
	if req.UserId == actor.ID {
		return nil, status.Error(codes.PermissionDenied, "cannot ban yourself")
	}

	user, err := s.AdminRepo.GetUserDetail(ctx, req.UserId)
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if user.Status == "banned" {
		return nil, status.Error(codes.FailedPrecondition, "user is already banned")
	}

	if err := s.UserRepo.UpdateRoleAndStatus(ctx, user.ID, user.Role, "banned"); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to ban user: %v", err)
	}

	// Invalidate live sessions and API keys via Redis (25h TTL outlives the
	// 24h max token lifetime).
	if s.Redis != nil {
		now := time.Now().Unix()
		_ = s.Redis.Set(ctx, "user_banned:"+user.ID, "1", adminBanKeyTTL)
		_ = s.Redis.Set(ctx, "token_revoked_before:"+user.ID, fmt.Sprintf("%d", now), adminBanKeyTTL)
	}

	// Pre-publication review (FR-2/US-8): re-gate the user's next M publishes
	// after reinstatement. Best-effort so a review-store failure never
	// blocks the ban itself.
	if s.ReviewRepo != nil {
		regate := defaultReviewSettings[settingRegateCount]
		if s.promptSvc != nil {
			regate = s.promptSvc.getSettingInt(ctx, settingRegateCount)
		}
		if err := s.ReviewRepo.SetGatedRemaining(ctx, user.ID, int32(regate)); err != nil {
			zap.S().Warnf("BanUser: failed to set review_gated_remaining for %s: %v", user.ID, err)
		}
	}

	s.writeAudit(ctx, actor, "ban", "user", user.ID, req.Reason)

	user.Status = "banned"
	return &pb.UserActionResponse{User: userToSummary(user, 0)}, nil
}

// UnbanUser lifts a ban. Existing tokens stay revoked; the user must sign in
// again.
func (s *AdminService) UnbanUser(ctx context.Context, req *pb.UnbanUserRequest) (*pb.UserActionResponse, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateReason(req.Reason); err != nil {
		return nil, err
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	user, err := s.AdminRepo.GetUserDetail(ctx, req.UserId)
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if user.Status != "banned" {
		return nil, status.Error(codes.FailedPrecondition, "user is not banned")
	}

	if err := s.UserRepo.UpdateRoleAndStatus(ctx, user.ID, user.Role, "active"); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unban user: %v", err)
	}

	// Only the ban marker is removed; tokens revoked before the ban stay
	// revoked so the user must re-authenticate.
	if s.Redis != nil {
		_ = s.Redis.Del(ctx, "user_banned:"+user.ID)
	}

	s.writeAudit(ctx, actor, "unban", "user", user.ID, req.Reason)

	user.Status = "active"
	return &pb.UserActionResponse{User: userToSummary(user, 0)}, nil
}

// ChangeUserRole promotes or demotes a user (self-demotion rejected).
func (s *AdminService) ChangeUserRole(ctx context.Context, req *pb.ChangeUserRoleRequest) (*pb.UserActionResponse, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateReason(req.Reason); err != nil {
		return nil, err
	}
	if req.Role != "admin" && req.Role != "user" {
		return nil, status.Error(codes.InvalidArgument, "role must be admin or user")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}
	if req.UserId == actor.ID && req.Role != "admin" {
		return nil, status.Error(codes.PermissionDenied, "cannot demote yourself")
	}

	user, err := s.AdminRepo.GetUserDetail(ctx, req.UserId)
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if user.Role == req.Role {
		return nil, status.Error(codes.FailedPrecondition, "user already has this role")
	}

	if err := s.UserRepo.UpdateRoleAndStatus(ctx, user.ID, req.Role, user.Status); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to change role: %v", err)
	}

	// Role changes revoke existing tokens so stale role claims cannot be used.
	if s.Redis != nil {
		now := time.Now().Unix()
		_ = s.Redis.Set(ctx, "token_revoked_before:"+user.ID, fmt.Sprintf("%d", now), adminBanKeyTTL)
	}

	s.writeAudit(ctx, actor, "role_change", "user", user.ID, req.Reason)

	user.Role = req.Role
	return &pb.UserActionResponse{User: userToSummary(user, 0)}, nil
}

// ---- templates ----

// ListTemplates returns a paginated moderation list of ALL templates.
func (s *AdminService) ListTemplates(ctx context.Context, req *pb.ListAdminTemplatesRequest) (*pb.ListAdminTemplatesResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}

	if req.Visibility != "" && req.Visibility != "public" && req.Visibility != "private" {
		return nil, status.Error(codes.InvalidArgument, "invalid visibility filter")
	}
	switch req.Sort {
	case "", "recently_updated", "newest", "oldest", "most_likes":
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid sort")
	}

	// Validate the warning filter: "" (any), "none", or a taxonomy key.
	if req.Warning != "" && req.Warning != "none" {
		taxonomy := s.taxonomy(ctx)
		if !slices.Contains(taxonomy, req.Warning) {
			return nil, status.Error(codes.InvalidArgument, "invalid warning filter")
		}
	}

	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	perPage := int(req.PerPage)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	templates, total, err := s.AdminRepo.ListTemplates(ctx, repository.TemplateFilters{
		Search:     strings.TrimSpace(req.Search),
		Visibility: req.Visibility,
		OwnerID:    req.OwnerId,
		Sort:       req.Sort,
		Warning:    req.Warning,
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list templates: %v", err)
	}

	resp := &pb.ListAdminTemplatesResponse{Total: total, Page: int32(page), PerPage: int32(perPage)}
	for _, t := range templates {
		resp.Templates = append(resp.Templates, templateToSummary(t))
	}
	return resp, nil
}

// GetTemplate returns any template with all versions (read-only).
func (s *AdminService) GetTemplate(ctx context.Context, req *pb.GetAdminTemplateRequest) (*pb.GetAdminTemplateResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "template id is required")
	}

	template, versions, err := s.AdminRepo.GetTemplateWithVersions(ctx, req.Id)
	if err != nil {
		if err == repository.ErrTemplateNotFound {
			return nil, status.Error(codes.NotFound, "template not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get template: %v", err)
	}

	resp := &pb.GetAdminTemplateResponse{
		Template:    templateToSummary(template),
		Description: template.Description.String,
	}
	for _, v := range versions {
		resp.Versions = append(resp.Versions, &pb.AdminTemplateVersion{
			Id:         v.ID,
			Version:    v.Version,
			Content:    v.Content,
			CreatedAt:  timestamppb.New(v.CreatedAt),
			AliasName:  v.AliasName,
			ChangeNote: v.ChangeNote.String,
		})
	}
	return resp, nil
}

// ---- audit ----

// ListAuditLogs returns the append-only audit trail.
func (s *AdminService) ListAuditLogs(ctx context.Context, req *pb.ListAuditLogsRequest) (*pb.ListAuditLogsResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}

	if req.Action != "" && req.Action != "ban" && req.Action != "unban" && req.Action != "role_change" && req.Action != "delete_comment" && req.Action != "review_approve" && req.Action != "review_reject" && req.Action != "template.warning_add" && req.Action != "template.warning_remove" {
		return nil, status.Error(codes.InvalidArgument, "invalid action filter")
	}

	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	perPage := int(req.PerPage)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	entries, total, err := s.AdminRepo.ListAuditLogs(ctx, repository.AuditFilters{
		Action: req.Action,
		Actor:  strings.TrimSpace(req.Actor),
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list audit logs: %v", err)
	}

	resp := &pb.ListAuditLogsResponse{Total: total, Page: int32(page), PerPage: int32(perPage)}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, auditLogToProto(e))
	}
	return resp, nil
}

// auditLogToProto converts a models.AdminAuditLog to the proto entry.
func auditLogToProto(e *models.AdminAuditLog) *pb.AdminAuditLogEntry {
	return &pb.AdminAuditLogEntry{
		Id:         e.ID,
		ActorId:    e.ActorID,
		ActorEmail: e.ActorEmail,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetId:   e.TargetID,
		Reason:     e.Reason,
		CreatedAt:  timestamppb.New(e.CreatedAt),
	}
}

// BootstrapAdmin promotes or creates the bootstrap admin account at startup.
// It is a no-op when ADMIN_BOOTSTRAP_EMAIL is empty.
func BootstrapAdmin(ctx context.Context, userRepo repository.UserRepository, email, password string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil
	}
	if len(password) < 8 {
		return fmt.Errorf("ADMIN_BOOTSTRAP_PASSWORD must be at least 8 characters when bootstrap is enabled")
	}

	user, err := userRepo.GetByEmail(ctx, email)
	if err == nil {
		if user.Role != "admin" || user.Status != "active" {
			if err := userRepo.UpdateRoleAndStatus(ctx, user.ID, "admin", "active"); err != nil {
				return fmt.Errorf("failed to promote bootstrap admin: %w", err)
			}
		}
		zap.S().Infof("Admin bootstrap: existing account %s promoted to active admin", email)
		return nil
	}
	if err != repository.ErrUserNotFound {
		return fmt.Errorf("failed to look up bootstrap admin: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash bootstrap password: %w", err)
	}
	user = &models.User{
		ID:           "admin_bootstrap",
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  "Admin",
		Role:         "admin",
		Status:       "active",
	}
	if err := userRepo.Insert(ctx, user); err != nil {
		return fmt.Errorf("failed to create bootstrap admin: %w", err)
	}
	zap.S().Infof("Admin bootstrap: created admin account %s", email)
	return nil
}

// ListAdminComments lists all comments platform-wide for moderation.
func (s *AdminService) ListAdminComments(ctx context.Context, req *pb.ListAdminCommentsRequest) (*pb.ListAdminCommentsResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	perPage := int(req.PerPage)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	entries, total, err := s.AdminRepo.ListComments(ctx, repository.CommentFilters{
		Search:     strings.TrimSpace(req.Q),
		TemplateID: strings.TrimSpace(req.TemplateId),
		UserID:     strings.TrimSpace(req.UserId),
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list comments: %v", err)
	}

	resp := &pb.ListAdminCommentsResponse{
		Entries: make([]*pb.AdminCommentEntry, 0, len(entries)),
		Total:   total,
		Page:    int32(page),
		PerPage: int32(perPage),
	}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &pb.AdminCommentEntry{
			Id:                       e.ID,
			TemplateId:               e.TemplateID,
			TemplateTitle:            e.TemplateTitle,
			TemplateOwnerId:          e.TemplateOwnerID,
			TemplateOwnerDisplayName: e.OwnerDisplayName,
			UserId:                   e.UserID,
			UserDisplayName:          e.UserDisplayName,
			UserEmail:                e.UserEmail,
			Content:                  e.Content,
			CreatedAt:                timestamppb.New(e.CreatedAt),
			UpdatedAt:                timestamppb.New(e.UpdatedAt),
		})
	}
	return resp, nil
}

// DeleteAdminComment deletes any comment with a mandatory reason (audited).
func (s *AdminService) DeleteAdminComment(ctx context.Context, req *pb.DeleteAdminCommentRequest) (*pb.DeleteAdminCommentResponse, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateReason(req.Reason); err != nil {
		return nil, err
	}
	if req.CommentId == "" {
		return nil, status.Error(codes.InvalidArgument, "comment id is required")
	}

	deleted, _, err := s.AdminRepo.DeleteComment(ctx, req.CommentId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete comment: %v", err)
	}
	if !deleted {
		return nil, status.Error(codes.NotFound, "comment not found")
	}

	s.writeAudit(ctx, actor, "delete_comment", "comment", req.CommentId, req.Reason)
	return &pb.DeleteAdminCommentResponse{Success: true}, nil
}

// ---- Pre-publication review ----

// validRejectReasonCodes is the server-side whitelist of rejection reason
// codes (constraint 9: the client never defines the set).
var validRejectReasonCodes = map[string]bool{
	"spam":       true,
	"plagiarism": true,
	"harmful":    true,
	"lowQuality": true,
	"other":      true,
}

// reviewAgingHours resolves the aging window (platform setting, cached in
// the prompt service) for queue ordering and the aging flag.
func (s *AdminService) reviewAgingHours(ctx context.Context) int {
	if s.promptSvc != nil {
		return s.promptSvc.getSettingInt(ctx, settingAgingHours)
	}
	return defaultReviewSettings[settingAgingHours]
}

// ListReviewQueue returns the pending review queue, aging-first then FIFO.
func (s *AdminService) ListReviewQueue(ctx context.Context, req *pb.ListReviewQueueRequest) (*pb.ListReviewQueueResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	if s.ReviewRepo == nil {
		return nil, status.Error(codes.Unimplemented, "review repository not configured")
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	rows, total, err := s.ReviewRepo.ListQueue(ctx, repository.ReviewQueueFilters{
		Category:         strings.TrimSpace(req.Category),
		AgingOnly:        req.AgingOnly,
		ResubmissionOnly: req.ResubmissionOnly,
		AgingHours:       s.reviewAgingHours(ctx),
		Limit:            int(perPage),
		Offset:           int((page - 1) * perPage),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list review queue: %v", err)
	}

	items := make([]*pb.ReviewQueueItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, &pb.ReviewQueueItem{
			Id:                     r.ID,
			TemplateId:             r.TemplateID,
			TemplateTitle:          r.TemplateTitle,
			CreatorId:              r.CreatorID,
			CreatorDisplayName:     r.CreatorName,
			AttemptNumber:          r.AttemptNumber,
			IsResubmission:         r.AttemptNumber >= 2,
			SubmittedAt:            timestamppb.New(r.SubmittedAt),
			Category:               r.TemplateCategory,
			Aging:                  r.Aging,
			CreatorApprovedCount:   r.CreatorApproved,
			CreatorPriorRejections: r.PriorRejections,
			CreatorBanned:          r.CreatorBanned,
		})
	}
	return &pb.ListReviewQueueResponse{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}, nil
}

// GetReviewSubmission returns the full detail for one submission.
func (s *AdminService) GetReviewSubmission(ctx context.Context, req *pb.GetReviewSubmissionRequest) (*pb.GetReviewSubmissionResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	if s.ReviewRepo == nil {
		return nil, status.Error(codes.Unimplemented, "review repository not configured")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "submission id is required")
	}

	d, err := s.ReviewRepo.GetSubmissionDetail(ctx, req.Id)
	if err != nil {
		if err == repository.ErrSubmissionNotFound {
			return nil, status.Error(codes.NotFound, "submission not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load submission: %v", err)
	}

	chain := make([]*pb.ReviewChainEntry, 0, len(d.History))
	for _, h := range d.History {
		entry := &pb.ReviewChainEntry{
			AttemptNumber: h.AttemptNumber,
			Status:        h.Status,
			SubmittedAt:   timestamppb.New(h.SubmittedAt),
		}
		if h.DecidedAt.Valid {
			entry.DecidedAt = timestamppb.New(h.DecidedAt.Time)
		}
		if h.RejectReasonCode.Valid {
			entry.RejectReasonCode = h.RejectReasonCode.String
		}
		if h.RejectNote.Valid {
			entry.RejectNote = h.RejectNote.String
		}
		chain = append(chain, entry)
	}

	return &pb.GetReviewSubmissionResponse{
		Submission: &pb.ReviewSubmissionDetail{
			Id:                     d.ID,
			TemplateId:             d.TemplateID,
			TemplateTitle:          d.TemplateTitle,
			TemplateDescription:    d.TemplateDescription.String,
			TemplateContent:        d.VersionContent,
			Tags:                   d.TemplateTags,
			Category:               d.TemplateCategory.String,
			Language:               d.TemplateLanguage,
			CreatorId:              d.CreatorID,
			CreatorDisplayName:     d.CreatorName,
			CreatorJoinedAt:        timestamppb.New(d.CreatorCreatedAt),
			CreatorPublishedCount:  d.CreatorApproved,
			CreatorPriorRejections: d.PriorRejections,
			CreatorBanned:          d.CreatorBanned,
			AttemptNumber:          d.AttemptNumber,
			SubmittedAt:            timestamppb.New(d.SubmittedAt),
			Chain:                  chain,
		},
	}, nil
}

// ApproveReviewSubmission approves a pending submission: the template goes
// live (moderation_status=normal, published_at=now), the creator's approved
// count increments, and a template_approved notification is emitted
// (best-effort).
func (s *AdminService) ApproveReviewSubmission(ctx context.Context, req *pb.ApproveReviewSubmissionRequest) (*pb.ReviewDecisionResponse, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.ReviewRepo == nil {
		return nil, status.Error(codes.Unimplemented, "review repository not configured")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "submission id is required")
	}

	sub, err := s.ReviewRepo.DecideSubmission(ctx, req.Id, "approved", actor.ID, nil, nil)
	if err != nil {
		return nil, s.mapDecisionError(err)
	}

	// Publish the template. The WHERE moderation_status='in_review' guard
	// makes a double-approve a no-op at the template level.
	if err := s.AdminRepo.SetTemplateModeration(ctx, sub.TemplateID, "normal", true); err != nil {
		zap.S().Errorf("ApproveReviewSubmission: failed to publish template %s: %v", sub.TemplateID, err)
		return nil, status.Errorf(codes.Internal, "failed to publish template: %v", err)
	}
	if err := s.ReviewRepo.IncrementApprovedCount(ctx, sub.CreatorID); err != nil {
		zap.S().Warnf("ApproveReviewSubmission: failed to increment approved count for %s: %v", sub.CreatorID, err)
	}

	s.writeAudit(ctx, actor, "review_approve", "template", sub.TemplateID, "approved in pre-publication review")
	s.notifyReviewDecision(ctx, actor, sub.CreatorID, sub.TemplateID, "template_approved", "")

	return &pb.ReviewDecisionResponse{
		SubmissionId: sub.ID,
		TemplateId:   sub.TemplateID,
		Status:       "approved",
	}, nil
}

// RejectReviewSubmission rejects a pending submission with a mandatory
// whitelisted reason code; the template is hidden (moderation_status=rejected)
// and the creator is notified with the reason (best-effort).
func (s *AdminService) RejectReviewSubmission(ctx context.Context, req *pb.RejectReviewSubmissionRequest) (*pb.ReviewDecisionResponse, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.ReviewRepo == nil {
		return nil, status.Error(codes.Unimplemented, "review repository not configured")
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "submission id is required")
	}
	reasonCode := strings.TrimSpace(req.ReasonCode)
	if !validRejectReasonCodes[reasonCode] {
		return nil, status.Error(codes.InvalidArgument, "reason_code must be one of: spam, plagiarism, harmful, lowQuality, other")
	}
	note := strings.TrimSpace(req.Note)
	if len(note) > 500 {
		return nil, status.Error(codes.InvalidArgument, "note must be at most 500 characters")
	}

	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	sub, err := s.ReviewRepo.DecideSubmission(ctx, req.Id, "rejected", actor.ID, &reasonCode, notePtr)
	if err != nil {
		return nil, s.mapDecisionError(err)
	}

	// Hide the template. The WHERE moderation_status='in_review' guard
	// keeps concurrent decisions consistent.
	if err := s.AdminRepo.SetTemplateModeration(ctx, sub.TemplateID, "rejected", false); err != nil {
		zap.S().Errorf("RejectReviewSubmission: failed to hide template %s: %v", sub.TemplateID, err)
		return nil, status.Errorf(codes.Internal, "failed to update template: %v", err)
	}

	reason := reasonCode
	if note != "" {
		reason = reasonCode + ": " + note
	}
	s.writeAudit(ctx, actor, "review_reject", "template", sub.TemplateID, reason)
	s.notifyReviewDecision(ctx, actor, sub.CreatorID, sub.TemplateID, "template_rejected", reasonCode)

	return &pb.ReviewDecisionResponse{
		SubmissionId: sub.ID,
		TemplateId:   sub.TemplateID,
		Status:       "rejected",
	}, nil
}

// mapDecisionError maps repository decision errors to gRPC codes.
func (s *AdminService) mapDecisionError(err error) error {
	if err == repository.ErrAlreadyDecided {
		return status.Error(codes.FailedPrecondition, "submission already decided")
	}
	if err == repository.ErrSubmissionNotFound {
		return status.Error(codes.NotFound, "submission not found")
	}
	return status.Errorf(codes.Internal, "failed to decide submission: %v", err)
}

// notifyReviewDecision emits the creator notification for a review
// decision. Best-effort: failures are logged, never fatal (FR-7/FR-11).
// Actor is the deciding admin (detailed design §3.6); self-decisions are
// suppressed (no_self_notify backstop). For rejections the reason code is
// carried in the actor_display_name slot so the client can render the
// actor-less sentence "rejected: <reason>" (review.notify.rejected).
func (s *AdminService) notifyReviewDecision(ctx context.Context, actor *models.User, creatorID, templateID, notificationType, reasonCode string) {
	if s.NotificationRepo == nil {
		return
	}
	if actor == nil || actor.ID == creatorID {
		// Self-decision (an admin reviewing their own submission): skip.
		return
	}
	displayName := reasonCode
	if displayName == "" {
		displayName = actor.DisplayName
		if displayName == "" {
			displayName = actor.ID
		}
	}
	if err := s.NotificationRepo.Create(ctx, &models.Notification{
		RecipientID:      creatorID,
		ActorID:          actor.ID,
		TemplateID:       templateID,
		Type:             notificationType,
		ActorDisplayName: displayName,
	}); err != nil {
		zap.S().Errorf("review decision notification: failed to create %s notification (template=%s creator=%s): %v",
			notificationType, templateID, creatorID, err)
	}
}

// GetReviewMetrics returns queue KPIs.
func (s *AdminService) GetReviewMetrics(ctx context.Context, _ *pb.GetReviewMetricsRequest) (*pb.ReviewMetrics, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	if s.ReviewRepo == nil {
		return nil, status.Error(codes.Unimplemented, "review repository not configured")
	}

	m, err := s.ReviewRepo.GetMetrics(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get review metrics: %v", err)
	}
	rates := make([]*pb.RejectionRateEntry, 0, len(m.RejectionRates))
	for _, e := range m.RejectionRates {
		rates = append(rates, &pb.RejectionRateEntry{
			ReasonCode: e.ReasonCode,
			Count:      e.Count,
			Rate:       e.Rate,
		})
	}
	return &pb.ReviewMetrics{
		OpenCount:               int64(m.PendingCount),
		MedianTimeInReviewHours: m.MedianHours,
		DecisionsToday:          int64(m.ApprovedToday + m.RejectedToday),
		RejectionRateByReason:   rates,
	}, nil
}

// GetPlatformSettings lists the admin-editable platform settings.
func (s *AdminService) GetPlatformSettings(ctx context.Context, _ *pb.GetPlatformSettingsRequest) (*pb.GetPlatformSettingsResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	if s.ReviewRepo == nil {
		return nil, status.Error(codes.Unimplemented, "review repository not configured")
	}

	settings, err := s.ReviewRepo.GetSettings(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get platform settings: %v", err)
	}
	entries := make([]*pb.PlatformSettingEntry, 0, len(settings))
	for _, st := range settings {
		entries = append(entries, &pb.PlatformSettingEntry{
			Key:       st.Key,
			Value:     st.Value,
			UpdatedAt: timestamppb.New(st.UpdatedAt),
		})
	}
	return &pb.GetPlatformSettingsResponse{Settings: entries}, nil
}

// UpdatePlatformSetting updates one platform setting with a mandatory
// audit-logged reason, then invalidates the prompt service's settings cache.
func (s *AdminService) UpdatePlatformSetting(ctx context.Context, req *pb.UpdatePlatformSettingRequest) (*pb.PlatformSettingEntry, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.ReviewRepo == nil {
		return nil, status.Error(codes.Unimplemented, "review repository not configured")
	}
	if err := validateReason(req.Reason); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(req.Key)
	if key != settingApprovalThreshold && key != settingRegateCount && key != settingAgingHours && key != settingWarningTaxonomy {
		return nil, status.Errorf(codes.InvalidArgument, "unknown setting key: %s", key)
	}
	// The taxonomy is a JSON array of keys; the numeric settings are integers.
	if key == settingWarningTaxonomy {
		var keys []string
		if err := json.Unmarshal([]byte(strings.TrimSpace(req.Value)), &keys); err != nil || len(keys) == 0 {
			return nil, status.Error(codes.InvalidArgument, "value must be a non-empty JSON array of warning keys")
		}
		if err := s.ReviewRepo.UpsertSetting(ctx, key, strings.TrimSpace(req.Value)); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to update setting: %v", err)
		}
		if s.promptSvc != nil {
			s.promptSvc.InvalidateSettingsCache()
		}
		s.writeAudit(ctx, actor, "settings_change", "platform", key, req.Reason)
		return &pb.PlatformSettingEntry{
			Key:       key,
			Value:     strings.TrimSpace(req.Value),
			UpdatedAt: timestamppb.New(time.Now()),
		}, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(req.Value))
	if err != nil || value < 0 {
		return nil, status.Error(codes.InvalidArgument, "value must be a non-negative integer")
	}
	// review_approval_threshold = 0 disables gating entirely (every publish
	// goes live); the other settings are meaningless at 0.
	if value == 0 && key != settingApprovalThreshold {
		return nil, status.Error(codes.InvalidArgument, "value must be a positive integer")
	}

	if err := s.ReviewRepo.UpsertSetting(ctx, key, strconv.Itoa(value)); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update setting: %v", err)
	}

	// Apply immediately: drop the cached settings in the prompt service.
	if s.promptSvc != nil {
		s.promptSvc.InvalidateSettingsCache()
	}

	s.writeAudit(ctx, actor, "settings_change", "platform", key, req.Reason)

	return &pb.PlatformSettingEntry{
		Key:       key,
		Value:     strconv.Itoa(value),
		UpdatedAt: timestamppb.New(time.Now()),
	}, nil
}

// UpdateTemplateWarnings adds/removes content warnings on a template with a
// mandatory reason. The change is audited (template.warning_add /
// template.warning_remove) and the creator is notified (best-effort).
func (s *AdminService) UpdateTemplateWarnings(ctx context.Context, req *pb.UpdateTemplateWarningsRequest) (*pb.UpdateTemplateWarningsResponse, error) {
	actor, err := s.requireAdminContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateReason(req.Reason); err != nil {
		return nil, err
	}
	if req.TemplateId == "" {
		return nil, status.Error(codes.InvalidArgument, "template id is required")
	}

	// Validate every key in add/remove against the taxonomy (constraint 8).
	taxonomy := s.taxonomy(ctx)
	for _, w := range req.Add {
		if !slices.Contains(taxonomy, w) {
			return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
		}
	}
	for _, w := range req.Remove {
		if !slices.Contains(taxonomy, w) {
			return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
		}
	}

	// Load the current template to compute the new sets.
	template, _, err := s.AdminRepo.GetTemplateWithVersions(ctx, req.TemplateId)
	if err != nil {
		if err == repository.ErrTemplateNotFound {
			return nil, status.Error(codes.NotFound, "template not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get template: %v", err)
	}

	current := template.ContentWarnings
	locked := template.LockedWarnings

	// newContent = (current ∪ add) − remove
	// newLocked = (currentLocked ∪ add) − remove (a moderator can remove any
	// warning they or another moderator applied).
	newContent := unionMinus(current, req.Add, req.Remove)
	newLocked := unionMinus(locked, req.Add, req.Remove)

	if err := s.AdminRepo.UpdateTemplateWarnings(ctx, req.TemplateId, newContent, newLocked); err != nil {
		if err == repository.ErrTemplateNotFound {
			return nil, status.Error(codes.NotFound, "template not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to update template warnings: %v", err)
	}

	// Audit each added/removed warning (best-effort). The reason field
	// carries a JSON array of the changed warnings so the history list can
	// reconstruct them.
	for _, w := range req.Add {
		payload, _ := json.Marshal([]string{w})
		s.writeAudit(ctx, actor, "template.warning_add", "template", req.TemplateId, string(payload))
	}
	for _, w := range req.Remove {
		payload, _ := json.Marshal([]string{w})
		s.writeAudit(ctx, actor, "template.warning_remove", "template", req.TemplateId, string(payload))
	}

	// Notify the creator (best-effort, self-suppressed). The
	// ActorDisplayName slot carries a compact JSON delta payload
	// {"added":[...],"removed":[...]} that the frontend renders into the
	// localized sentence (OQ-5).
	s.notifyWarningChange(ctx, actor, template.OwnerID, req.TemplateId, req.Add, req.Remove)

	return &pb.UpdateTemplateWarningsResponse{
		TemplateId:      req.TemplateId,
		ContentWarnings: newContent,
		LockedWarnings:  newLocked,
	}, nil
}

// unionMinus computes (base ∪ add) − remove, preserving order and deduping.
func unionMinus(base, add, remove []string) []string {
	removeSet := make(map[string]bool, len(remove))
	for _, r := range remove {
		removeSet[r] = true
	}
	seen := make(map[string]bool)
	var out []string
	for _, w := range append(append([]string{}, base...), add...) {
		if removeSet[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// notifyWarningChange emits the template_warning_changed notification to the
// template owner. Best-effort: failures are logged, never fatal. Self-actions
// (an admin changing warnings on their own template) are suppressed.
func (s *AdminService) notifyWarningChange(ctx context.Context, actor *models.User, ownerID, templateID string, added, removed []string) {
	if s.NotificationRepo == nil {
		return
	}
	if actor == nil || actor.ID == ownerID {
		// Self-decision (an admin changing warnings on their own template): skip.
		zap.S().Warnf("warning change notification: self-action suppressed (template=%s)", templateID)
		return
	}
	payload, err := json.Marshal(map[string][]string{"added": added, "removed": removed})
	if err != nil {
		zap.S().Errorf("warning change notification: failed to marshal delta: %v", err)
		return
	}
	if err := s.NotificationRepo.Create(ctx, &models.Notification{
		RecipientID:      ownerID,
		ActorID:          actor.ID,
		TemplateID:       templateID,
		Type:             "template_warning_changed",
		ActorDisplayName: string(payload),
	}); err != nil {
		zap.S().Errorf("warning change notification: failed to create notification (template=%s creator=%s): %v",
			templateID, ownerID, err)
	}
}

// ListWarningHistory returns the warning-change history for a template,
// newest first, paginated.
func (s *AdminService) ListWarningHistory(ctx context.Context, req *pb.ListWarningHistoryRequest) (*pb.ListWarningHistoryResponse, error) {
	if _, err := s.requireAdminContext(ctx); err != nil {
		return nil, err
	}
	if req.TemplateId == "" {
		return nil, status.Error(codes.InvalidArgument, "template id is required")
	}

	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	perPage := int(req.PerPage)
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	entries, total, err := s.AdminRepo.ListWarningHistory(ctx, req.TemplateId, perPage, (page-1)*perPage)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list warning history: %v", err)
	}

	resp := &pb.ListWarningHistoryResponse{Total: total, Page: int32(page), PerPage: int32(perPage)}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, &pb.WarningHistoryEntry{
			Id:         e.ID,
			ActorId:    e.ActorID,
			ActorEmail: e.ActorEmail,
			Action:     e.Action,
			Warnings:   e.Warnings,
			Reason:     e.Reason,
			CreatedAt:  timestamppb.New(e.CreatedAt),
		})
	}
	return resp, nil
}
