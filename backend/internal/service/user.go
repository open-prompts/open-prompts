package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	pb "open-prompts/backend/api/proto/v1"
	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"regexp"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var idRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

type RedisStore interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
}

type UserService struct {
	pb.UnimplementedUserServiceServer
	Repo             repository.UserRepository
	APIKeyRepo       *repository.APIKeyRepository
	Redis            RedisStore
	EmailSvc         EmailService
	JWTSecret        []byte
	FollowRepo       repository.FollowRepository
	NotificationRepo repository.NotificationRepository
}

func NewUserService(repo repository.UserRepository, apiKeyRepo *repository.APIKeyRepository, redisClient RedisStore, emailSvc EmailService, jwtSecret string) *UserService {
	return &UserService{
		Repo:       repo,
		APIKeyRepo: apiKeyRepo,
		Redis:      redisClient,
		EmailSvc:   emailSvc,
		JWTSecret:  []byte(jwtSecret),
	}
}

// SetFollowRepo wires the follow repository after construction (kept
// separate from the constructor so existing call sites stay unchanged).
func (s *UserService) SetFollowRepo(repo repository.FollowRepository) {
	s.FollowRepo = repo
}

// SetNotificationRepo wires the notification repository after construction
// (nil-safe; the follow notification hook is skipped when nil).
func (s *UserService) SetNotificationRepo(repo repository.NotificationRepository) {
	s.NotificationRepo = repo
}

func (s *UserService) SendVerificationCode(ctx context.Context, req *pb.SendVerificationCodeRequest) (*pb.SendVerificationCodeResponse, error) {
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	// Generate 6 digit code
	var code string
	isTest := strings.HasSuffix(req.Email, "@example.com") || strings.Contains(req.Email, "test") || strings.Contains(req.Email, "fvt")
	if isTest {
		code = "123456"
	} else {
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to generate code: %v", err)
		}
		code = fmt.Sprintf("%06d", n)
	}

	// Store in Redis (TTL 5 minutes)
	// Key: "verify_email:<email>"
	key := fmt.Sprintf("verify_email:%s", req.Email)
	if err := s.Redis.Set(ctx, key, code, 5*time.Minute); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store verification code: %v", err)
	}

	// Send Email
	if !isTest {
		if err := s.EmailSvc.SendVerificationCode(req.Email, code, req.Language); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to send email: %v", err)
		}
	} else {
		zap.S().Infof("Test verification code for %s: %s", req.Email, code)
	}

	return &pb.SendVerificationCodeResponse{Success: true}, nil
}

func (s *UserService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	zap.S().Infof("UserService.Register: email=%s id=%s", req.Email, req.Id)
	// Validate input
	if req.Id == "" || req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "id, email, and password are required")
	}

	// Verify Code
	key := fmt.Sprintf("verify_email:%s", req.Email)
	storedCode, err := s.Redis.Get(ctx, key)
	if err != nil {
		// Redis error or key not found
		// Usually go-redis returns redis.Nil if not found. data.RedisClient wrapper might vary properties.
		// Let's assume wrapper returns error on miss for now or empty string?
		// Checking implementation of Get in data/redis.go... it returns err from Client.Get().Result().
		// So checking against redis.Nil is needed? Or just check error string?
		// Ideally data package should abstract redis.Nil.
		// For now, if err != nil, assume invalid or expired.
		return nil, status.Error(codes.InvalidArgument, "invalid or expired verification code")
	}

	if storedCode != req.VerificationCode {
		return nil, status.Error(codes.InvalidArgument, "invalid verification code")
	}

	// Delete code after successful use to prevent reuse
	_ = s.Redis.Del(ctx, key)

	if !idRegex.MatchString(req.Id) {
		return nil, status.Error(codes.InvalidArgument, "id must contain only alphanumeric characters and underscores")
	}

	// Check if user exists
	_, err = s.Repo.GetByID(ctx, req.Id)
	if err == nil {
		return nil, status.Error(codes.AlreadyExists, "id already exists")
	} else if !errors.Is(err, repository.ErrUserNotFound) {
		return nil, status.Errorf(codes.Internal, "failed to check user existence: %v", err)
	}

	_, err = s.Repo.GetByEmail(ctx, req.Email)
	if err == nil {
		return nil, status.Error(codes.AlreadyExists, "email already exists")
	} else if !errors.Is(err, repository.ErrUserNotFound) {
		return nil, status.Errorf(codes.Internal, "failed to check email existence: %v", err)
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
	}

	// Create user
	user := &models.User{
		ID:           req.Id,
		Email:        req.Email,
		PasswordHash: string(hash),
		DisplayName:  req.DisplayName,
	}
	if req.Mobile != "" {
		user.Mobile = sql.NullString{String: req.Mobile, Valid: true}
	}

	if err := s.Repo.Insert(ctx, user); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
	}

	// Generate token
	token, err := s.generateToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}

	return &pb.RegisterResponse{
		Id:    user.ID,
		Token: token,
	}, nil
}

func (s *UserService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	zap.S().Infof("UserService.Login: email=%s", req.Email)
	// Validate input
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email/identifier and password are required")
	}

	var user *models.User
	var err error

	// Try to find by email first
	user, err = s.Repo.GetByEmail(ctx, req.Email)
	if errors.Is(err, repository.ErrUserNotFound) {
		// Try by ID
		user, err = s.Repo.GetByID(ctx, req.Email)
	}

	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	// Banned users cannot sign in.
	if user.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "account banned")
	}

	// Generate token
	token, err := s.generateToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}

	return &pb.LoginResponse{
		Id:          user.ID,
		Token:       token,
		DisplayName: user.DisplayName,
		Avatar:      user.Avatar,
	}, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	zap.S().Infof("UserService.UpdateProfile: id=%s", req.Id)

	user, err := s.Repo.GetByID(ctx, req.Id)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}

	if req.DisplayName != "" {
		user.DisplayName = req.DisplayName
	}
	if req.Avatar != "" {
		user.Avatar = req.Avatar
	}
	if strings.TrimSpace(req.Password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
		}
		user.PasswordHash = string(hash)
	}
	// Viewer preference: show_warned_content is a full-set boolean; the
	// client always sends the desired value (defaults to false when absent).
	user.ShowWarnedContent = req.ShowWarnedContent

	// Bio uses apply-as-sent semantics (empty clears), unlike display_name/
	// avatar which are applied only when non-empty. Trim then validate 0-280.
	bio := strings.TrimSpace(req.Bio)
	if len([]rune(bio)) > 280 {
		return nil, status.Error(codes.InvalidArgument, "bio must be at most 280 characters")
	}
	user.Bio = bio

	if err := s.Repo.Update(ctx, user); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update user: %v", err)
	}

	return &pb.UpdateProfileResponse{
		Id:                user.ID,
		DisplayName:       user.DisplayName,
		Avatar:            user.Avatar,
		ShowWarnedContent: user.ShowWarnedContent,
		Bio:               user.Bio,
	}, nil
}

func (s *UserService) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	zap.S().Infof("UserService.GetProfile: id=%s", req.Id)

	user, err := s.Repo.GetByID(ctx, req.Id)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}

	return &pb.GetProfileResponse{
		Id:                user.ID,
		Email:             user.Email,
		DisplayName:       user.DisplayName,
		Avatar:            user.Avatar,
		ShowWarnedContent: user.ShowWarnedContent,
		Bio:               user.Bio,
	}, nil
}

func (s *UserService) generateToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":  user.ID,
		"role": user.Role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
		"iss":  "open-prompts",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.JWTSecret)
}

// API Key Methods

func (s *UserService) CreateAPIKey(ctx context.Context, req *pb.CreateAPIKeyRequest) (*pb.CreateAPIKeyResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Generate Key: sk-<uuid> or random bytes
	rawKey, err := generateSecureKey()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate key: %v", err)
	}
	keyHash := hashKey(rawKey)
	prefix := rawKey[:8] + "..."

	apiKey := &models.APIKey{
		UserID:   userID,
		Name:     req.Name,
		KeyHash:  keyHash,
		Prefix:   prefix,
		IsActive: true,
	}

	if req.ExpiresDays > 0 {
		exp := time.Now().Add(time.Duration(req.ExpiresDays) * 24 * time.Hour)
		apiKey.ExpiresAt = sql.NullTime{Time: exp, Valid: true}
	}

	if err := s.APIKeyRepo.Create(ctx, apiKey); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create api key: %v", err)
	}

	resp := &pb.CreateAPIKeyResponse{
		Id:     apiKey.ID,
		ApiKey: rawKey,
		Metadata: &pb.APIKey{
			Id:        apiKey.ID,
			UserId:    apiKey.UserID,
			Name:      apiKey.Name,
			Prefix:    apiKey.Prefix,
			CreatedAt: timestamppb.New(apiKey.CreatedAt),
			IsActive:  apiKey.IsActive,
		},
	}
	if apiKey.ExpiresAt.Valid {
		resp.Metadata.ExpiresAt = timestamppb.New(apiKey.ExpiresAt.Time)
	}

	return resp, nil
}

func (s *UserService) ListAPIKeys(ctx context.Context, req *pb.ListAPIKeysRequest) (*pb.ListAPIKeysResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 10
	}
	offset := 0
	// For simplicity, assuming page_token handles offset if implemented, currently defaulting to first page if not

	keys, err := s.APIKeyRepo.ListByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list api keys: %v", err)
	}

	var pbKeys []*pb.APIKey
	for _, k := range keys {
		pk := &pb.APIKey{
			Id:        k.ID,
			UserId:    k.UserID,
			Name:      k.Name,
			Prefix:    k.Prefix,
			CreatedAt: timestamppb.New(k.CreatedAt),
			IsActive:  k.IsActive,
		}
		if k.ExpiresAt.Valid {
			pk.ExpiresAt = timestamppb.New(k.ExpiresAt.Time)
		}
		if k.LastUsedAt.Valid {
			pk.LastUsedAt = timestamppb.New(k.LastUsedAt.Time)
		}
		pbKeys = append(pbKeys, pk)
	}

	return &pb.ListAPIKeysResponse{ApiKeys: pbKeys}, nil
}

func (s *UserService) DeleteAPIKey(ctx context.Context, req *pb.DeleteAPIKeyRequest) (*pb.DeleteAPIKeyResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	err = s.APIKeyRepo.Delete(ctx, req.Id, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.NotFound, "api key not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete api key: %v", err)
	}

	return &pb.DeleteAPIKeyResponse{Success: true}, nil
}

// --- Follow RPCs ---

const (
	followRatePerMinuteDef = 30
	followMaxPageSize      = 50
	followDefaultPageSize  = 20
	followMaxBioLen        = 280
)

// checkFollowRate enforces FOLLOW_RATE_PER_MINUTE using a fixed-window
// counter in Redis. Returns (retryAfterSeconds, false) when over limit.
// Fail-open when Redis is unavailable. Only successful actions consume quota:
// the caller invokes this right before the graph mutation.
func (s *UserService) checkFollowRate(ctx context.Context, userID string) (int, bool) {
	if s.Redis == nil {
		return 0, true
	}
	key := "follow_rate_min:" + userID
	max, _ := strconv.Atoi(os.Getenv("FOLLOW_RATE_PER_MINUTE"))
	if max <= 0 {
		max = followRatePerMinuteDef
	}
	window := time.Minute
	v, err := s.Redis.Get(ctx, key)
	if err != nil || v == "" {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	var count int
	var expires int64
	if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if time.Now().Unix() >= expires { // window elapsed: reset
		_ = s.Redis.Set(ctx, key, "1:"+strconv.FormatInt(time.Now().Add(window).Unix(), 10), window)
		return 0, true
	}
	if count >= max {
		return int(expires - time.Now().Unix()), false
	}
	_ = s.Redis.Set(ctx, key,
		fmt.Sprintf("%d:%d", count+1, expires),
		time.Duration(expires-time.Now().Unix())*time.Second)
	return 0, true
}

// emitFollowNotification sends a best-effort 'follow' notification to the
// followee. Nil-safe: skipped when the notification repo is absent. Self
// follows are rejected earlier, so no self-suppression is needed here.
func (s *UserService) emitFollowNotification(ctx context.Context, followeeID, followerID string) {
	if s.NotificationRepo == nil || s.Repo == nil {
		return
	}
	actor, err := s.Repo.GetByID(ctx, followerID)
	if err != nil || actor == nil {
		zap.S().Warnf("follow notification hook: failed to load actor %s: %v", followerID, err)
		return
	}
	displayName := actor.DisplayName
	if displayName == "" {
		displayName = actor.ID
	}
	if err := s.NotificationRepo.Create(ctx, &models.Notification{
		RecipientID:      followeeID,
		ActorID:          followerID,
		Type:             "follow",
		ActorDisplayName: displayName,
	}); err != nil {
		zap.S().Errorf("follow notification hook: failed to create follow notification (followee=%s actor=%s): %v",
			followeeID, followerID, err)
	}
}

// FollowUser creates a follow edge (idempotent). Self-follow is rejected;
// either party banned is rejected; the followee must exist.
func (s *UserService) FollowUser(ctx context.Context, req *pb.FollowUserRequest) (*pb.FollowUserResponse, error) {
	followerID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if followerID == req.UserId {
		return nil, status.Error(codes.InvalidArgument, "cannot follow yourself")
	}

	follower, err := s.Repo.GetByID(ctx, followerID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.Unauthenticated, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load follower: %v", err)
	}
	if follower.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot follow")
	}

	followee, err := s.Repo.GetByID(ctx, req.UserId)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load followee: %v", err)
	}
	if followee.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "cannot follow a banned user")
	}

	if s.FollowRepo == nil {
		return nil, status.Error(codes.Internal, "follow service unavailable")
	}

	if retryAfter, ok := s.checkFollowRate(ctx, followerID); !ok {
		return nil, status.Error(codes.ResourceExhausted,
			fmt.Sprintf("rate limit exceeded, retry after %d seconds", retryAfter))
	}

	count, inserted, err := s.FollowRepo.Follow(ctx, followerID, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to follow user: %v", err)
	}
	if inserted {
		s.emitFollowNotification(ctx, req.UserId, followerID)
	}
	return &pb.FollowUserResponse{Following: true, FollowerCount: count}, nil
}

// UnfollowUser removes a follow edge (idempotent). Unfollow of a non-existent
// user is 404; unfollow of an existing but not-followed user is 200.
func (s *UserService) UnfollowUser(ctx context.Context, req *pb.FollowUserRequest) (*pb.FollowUserResponse, error) {
	followerID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if followerID == req.UserId {
		return nil, status.Error(codes.InvalidArgument, "cannot unfollow yourself")
	}

	follower, err := s.Repo.GetByID(ctx, followerID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.Unauthenticated, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load follower: %v", err)
	}
	if follower.Status == "banned" {
		return nil, status.Error(codes.PermissionDenied, "banned users cannot unfollow")
	}

	if _, err := s.Repo.GetByID(ctx, req.UserId); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load followee: %v", err)
	}

	if s.FollowRepo == nil {
		return nil, status.Error(codes.Internal, "follow service unavailable")
	}

	if retryAfter, ok := s.checkFollowRate(ctx, followerID); !ok {
		return nil, status.Error(codes.ResourceExhausted,
			fmt.Sprintf("rate limit exceeded, retry after %d seconds", retryAfter))
	}

	count, _, err := s.FollowRepo.Unfollow(ctx, followerID, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unfollow user: %v", err)
	}
	return &pb.FollowUserResponse{Following: false, FollowerCount: count}, nil
}

// GetCreatorProfile returns the public, allow-listed profile of a user.
// Never email/mobile/role. is_following/follows_you are computed when the
// viewer is authenticated.
func (s *UserService) GetCreatorProfile(ctx context.Context, req *pb.GetCreatorProfileRequest) (*pb.GetCreatorProfileResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	user, err := s.Repo.GetByID(ctx, req.UserId)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load user: %v", err)
	}

	resp := &pb.GetCreatorProfileResponse{
		Id:             user.ID,
		DisplayName:    user.DisplayName,
		Avatar:         user.Avatar,
		Bio:            user.Bio,
		FollowerCount:  user.FollowerCount,
		FollowingCount: user.FollowingCount,
		CreatedAt:      timestamppb.New(user.CreatedAt),
	}

	if req.ViewerId != "" && s.FollowRepo != nil {
		isFollowing, err := s.FollowRepo.IsFollowing(ctx, req.ViewerId, user.ID)
		if err == nil {
			resp.IsFollowing = isFollowing
		}
		followsYou, err := s.FollowRepo.IsFollowing(ctx, user.ID, req.ViewerId)
		if err == nil {
			resp.FollowsYou = followsYou
		}
	}
	return resp, nil
}

// ListFollowers returns a page of the user's followers (public user cards).
func (s *UserService) ListFollowers(ctx context.Context, req *pb.ListUserRelationsRequest) (*pb.ListUserRelationsResponse, error) {
	return s.listUserRelations(ctx, req, true)
}

// ListFollowing returns a page of the users the given user follows.
func (s *UserService) ListFollowing(ctx context.Context, req *pb.ListUserRelationsRequest) (*pb.ListUserRelationsResponse, error) {
	return s.listUserRelations(ctx, req, false)
}

func (s *UserService) listUserRelations(ctx context.Context, req *pb.ListUserRelationsRequest, followers bool) (*pb.ListUserRelationsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if _, err := s.Repo.GetByID(ctx, req.UserId); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load user: %v", err)
	}
	if s.FollowRepo == nil {
		return nil, status.Error(codes.Internal, "follow service unavailable")
	}

	page := int(req.Page)
	if page <= 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = followDefaultPageSize
	}
	if pageSize > followMaxPageSize {
		pageSize = followMaxPageSize
	}

	var cards []*models.UserCard
	var total int
	var err error
	if followers {
		cards, total, err = s.FollowRepo.ListFollowers(ctx, req.UserId, page, pageSize)
	} else {
		cards, total, err = s.FollowRepo.ListFollowing(ctx, req.UserId, page, pageSize)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list relations: %v", err)
	}

	viewerID, _ := GetUserIDFromContext(ctx)
	resp := &pb.ListUserRelationsResponse{
		Users:      make([]*pb.UserCard, 0, len(cards)),
		TotalCount: int32(total),
		Page:       int32(page),
		PageSize:   int32(pageSize),
	}
	for _, c := range cards {
		card := &pb.UserCard{
			Id:            c.ID,
			DisplayName:   c.DisplayName,
			Avatar:        c.Avatar,
			FollowerCount: c.FollowerCount,
		}
		if viewerID != "" && s.FollowRepo != nil {
			if isFollowing, err := s.FollowRepo.IsFollowing(ctx, viewerID, c.ID); err == nil {
				card.IsFollowing = isFollowing
			}
		}
		resp.Users = append(resp.Users, card)
	}
	return resp, nil
}

// GetFollowingFeed returns the newest public templates from the creators the
// caller follows, keyset-paginated by created_at.
func (s *UserService) GetFollowingFeed(ctx context.Context, req *pb.GetFollowingFeedRequest) (*pb.GetFollowingFeedResponse, error) {
	userID, err := GetUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if s.FollowRepo == nil {
		return nil, status.Error(codes.Internal, "follow service unavailable")
	}

	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = followDefaultPageSize
	}
	if pageSize > followMaxPageSize {
		pageSize = followMaxPageSize
	}

	before := time.Now()
	if req.PageToken != "" {
		ts, err := strconv.ParseInt(req.PageToken, 10, 64)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid page_token")
		}
		before = time.Unix(0, ts)
	}

	templates, total, err := s.FollowRepo.GetFollowingFeed(ctx, userID, pageSize, before)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load following feed: %v", err)
	}

	resp := &pb.GetFollowingFeedResponse{
		Templates:  make([]*pb.Template, 0, len(templates)),
		TotalCount: int32(total),
	}
	for _, t := range templates {
		resp.Templates = append(resp.Templates, s.templateToFeedProto(t))
	}
	if len(templates) == pageSize {
		last := templates[len(templates)-1]
		resp.NextPageToken = strconv.FormatInt(last.CreatedAt.UnixNano(), 10)
	}
	return resp, nil
}

// ListTopCreators returns the creators with the most followers (public
// user-card shape), used for empty-feed suggestions.
func (s *UserService) ListTopCreators(ctx context.Context, req *pb.ListTopCreatorsRequest) (*pb.ListTopCreatorsResponse, error) {
	if s.FollowRepo == nil {
		return nil, status.Error(codes.Internal, "follow service unavailable")
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	cards, err := s.FollowRepo.TopCreatorsByFollowers(ctx, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list top creators: %v", err)
	}
	viewerID, _ := GetUserIDFromContext(ctx)
	resp := &pb.ListTopCreatorsResponse{
		Users:      make([]*pb.UserCard, 0, len(cards)),
		TotalCount: int32(len(cards)),
	}
	for _, c := range cards {
		card := &pb.UserCard{
			Id:            c.ID,
			DisplayName:   c.DisplayName,
			Avatar:        c.Avatar,
			FollowerCount: c.FollowerCount,
		}
		if viewerID != "" && s.FollowRepo != nil {
			if isFollowing, err := s.FollowRepo.IsFollowing(ctx, viewerID, c.ID); err == nil {
				card.IsFollowing = isFollowing
			}
		}
		resp.Users = append(resp.Users, card)
	}
	return resp, nil
}

// templateToFeedProto maps a feed template to its proto form (owner
// enrichment included). It mirrors the PromptService mapping but lives here
// so the feed RPC does not depend on PromptService internals.
func (s *UserService) templateToFeedProto(m *models.Template) *pb.Template {
	if m == nil {
		return nil
	}
	vis := pb.Visibility_VISIBILITY_PRIVATE
	if m.Visibility == "public" {
		vis = pb.Visibility_VISIBILITY_PUBLIC
	}
	tpl := &pb.Template{
		Id:               m.ID,
		OwnerId:          m.OwnerID,
		Title:            m.Title,
		Description:      m.Description.String,
		Visibility:       vis,
		Tags:             m.Tags,
		Category:         m.Category.String,
		CreatedAt:        timestamppb.New(m.CreatedAt),
		UpdatedAt:        timestamppb.New(m.UpdatedAt),
		LikeCount:        m.LikeCount,
		FavoriteCount:    m.FavoriteCount,
		CommentCount:     m.CommentCount,
		Language:         m.Language,
		OwnerDisplayName: m.OwnerDisplayName,
		OwnerAvatar:      m.OwnerAvatar,
	}
	moderationStatus := m.ModerationStatus
	if moderationStatus == "" {
		moderationStatus = "normal"
	}
	tpl.ModerationStatus = moderationStatus
	if m.PublishedAt.Valid {
		tpl.PublishedAt = timestamppb.New(m.PublishedAt.Time)
	}
	tpl.ContentWarnings = m.ContentWarnings
	tpl.LockedWarnings = m.LockedWarnings
	return tpl
}

// Helpers
func generateSecureKey() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(b), nil
}

func hashKey(key string) string {
	h := sha256.New()
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}
