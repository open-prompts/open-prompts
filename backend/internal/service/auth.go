package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"open-prompts/backend/internal/repository"

	"github.com/golang-jwt/jwt/v5"
)

// AuthInterceptor is a server interceptor that authenticates the user.
type AuthInterceptor struct {
	jwtSecret        []byte
	apiKeyRepo       *repository.APIKeyRepository
	userRepo         repository.UserRepository
	redis            RedisStore
	publicRpcMethods map[string]bool
}

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	userRoleKey  contextKey = "user_role"
	userScopeKey contextKey = "user_scope"
)

// ContextWithUserID adds the user ID to the context.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// ContextWithUserRole adds the user role to the context.
func ContextWithUserRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, userRoleKey, role)
}

// ContextWithUserScope adds the token scope to the context.
func ContextWithUserScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, userScopeKey, scope)
}

// GetUserRoleFromContext retrieves the user role from the context.
func GetUserRoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(userRoleKey).(string)
	return role
}

// GetUserScopeFromContext retrieves the token scope from the context.
func GetUserScopeFromContext(ctx context.Context) string {
	scope, _ := ctx.Value(userScopeKey).(string)
	return scope
}

// NewAuthInterceptor creates a new AuthInterceptor.
func NewAuthInterceptor(jwtSecret string, apiKeyRepo *repository.APIKeyRepository) *AuthInterceptor {
	return &AuthInterceptor{
		jwtSecret:  []byte(jwtSecret),
		apiKeyRepo: apiKeyRepo,
		publicRpcMethods: map[string]bool{
			"/v1.UserService/Register":         true,
			"/v1.UserService/Login":            true,
			"/v1.UserService/LoginWithOAuth":   true,
			"/v1.PromptService/ListTemplates":  true, // Allow public viewing? Maybe make it conditional essentially
			"/v1.PromptService/GetTemplate":    true,
			"/v1.PromptService/ListCategories": true,
			"/v1.PromptService/ListTags":       true,
			"/v1.AdminService/AdminLogin":      true,
			// For testing reflection
			"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": true,
		},
	}
}

// SetUserRepository wires the user repository used for ban/role fallback checks.
func (i *AuthInterceptor) SetUserRepository(repo repository.UserRepository) {
	i.userRepo = repo
}

// SetRedis wires the redis client used for ban/revocation checks.
func (i *AuthInterceptor) SetRedis(redis RedisStore) {
	i.redis = redis
}

// TokenClaims carries the verified identity claims of a JWT.
type TokenClaims struct {
	UserID string
	Role   string
	Scope  string
	Iat    int64
}

// VerifyToken validates the token string and returns the user ID.
// VerifyAnyToken verifies either an API key (if prefixed with sk-) or a JWT.
// For JWTs it also enforces the ban and revocation state from Redis (with a
// DB fallback); for API keys it rejects keys of banned users.
func (i *AuthInterceptor) VerifyAnyToken(ctx context.Context, tokenOrKey string) (string, error) {
	if strings.HasPrefix(tokenOrKey, "sk-") {
		return i.VerifyAPIKey(ctx, tokenOrKey)
	}
	claims, err := i.VerifyTokenWithClaims(tokenOrKey)
	if err != nil {
		return "", err
	}
	if err := i.checkBanAndRevocation(ctx, claims); err != nil {
		return "", err
	}
	return claims.UserID, nil
}

// VerifyToken validates the token string and returns the user ID.
func (i *AuthInterceptor) VerifyToken(tokenString string) (string, error) {
	claims, err := i.VerifyTokenWithClaims(tokenString)
	if err != nil {
		return "", err
	}
	return claims.UserID, nil
}

// VerifyTokenWithClaims validates the token and returns all identity claims.
func (i *AuthInterceptor) VerifyTokenWithClaims(tokenString string) (*TokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return i.jwtSecret, nil
	})

	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["sub"].(string)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid token payload: missing sub")
		}
		role, _ := claims["role"].(string)
		scope, _ := claims["scope"].(string)
		var iat int64
		switch v := claims["iat"].(type) {
		case float64:
			iat = int64(v)
		case int64:
			iat = v
		}
		return &TokenClaims{UserID: userID, Role: role, Scope: scope, Iat: iat}, nil
	}

	return nil, status.Error(codes.Unauthenticated, "invalid token")
}

// checkBanAndRevocation enforces the Redis ban/revocation keys for a verified
// JWT, falling back to a direct DB check when Redis is unavailable.
func (i *AuthInterceptor) checkBanAndRevocation(ctx context.Context, claims *TokenClaims) error {
	if i.redis != nil {
		banned, err := i.redis.Get(ctx, "user_banned:"+claims.UserID)
		if err == nil && banned != "" {
			return status.Error(codes.PermissionDenied, "account banned")
		}
		revokedBefore, err := i.redis.Get(ctx, "token_revoked_before:"+claims.UserID)
		if err == nil && revokedBefore != "" {
			if before, perr := strconv.ParseInt(revokedBefore, 10, 64); perr == nil && claims.Iat < before {
				return status.Error(codes.Unauthenticated, "token revoked")
			}
		}
		if err == nil {
			return nil
		}
		// Redis unavailable: fall through to the DB check.
	}
	return i.checkBanAndRevocationInDB(ctx, claims)
}

// checkBanAndRevocationInDB is the DB fallback when Redis is unavailable.
func (i *AuthInterceptor) checkBanAndRevocationInDB(ctx context.Context, claims *TokenClaims) error {
	if i.userRepo == nil {
		return nil
	}
	user, err := i.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return status.Error(codes.Unauthenticated, "invalid token")
	}
	if user.Status == "banned" {
		return status.Error(codes.PermissionDenied, "account banned")
	}
	return nil
}

// Unary returns a server interceptor function to authenticate unary RPCs.
func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 1. Attempt to extract and verify token or API Key
		var tokenString string
		var apiKeyString string

		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			zap.S().Infof("AuthInterceptor: md=%v", md)

			// Check Authorization header
			values := md["authorization"]
			if len(values) > 0 {
				authHeader := values[0]
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 {
					scheme := strings.ToLower(parts[0])
					switch scheme {
					case "bearer":
						tokenString = parts[1]
					case "apikey":
						apiKeyString = parts[1]
					}
				}
			}

			// Check X-API-Key header if Authorization not set or failed
			if apiKeyString == "" {
				apiKeys := md["x-api-key"]
				if len(apiKeys) > 0 {
					apiKeyString = apiKeys[0]
				}
			}
		}

		if tokenString != "" {
			// Token is present, verify it
			claims, err := i.VerifyTokenWithClaims(tokenString)
			if err != nil {
				return nil, err // Fail if token provided but invalid
			}
			if err := i.checkBanAndRevocation(ctx, claims); err != nil {
				return nil, err
			}
			// Inject User ID and role/scope claims into Context
			newCtx := ContextWithUserID(ctx, claims.UserID)
			newCtx = ContextWithUserRole(newCtx, claims.Role)
			newCtx = ContextWithUserScope(newCtx, claims.Scope)
			return handler(newCtx, req)
		}

		if apiKeyString != "" {
			userID, err := i.VerifyAPIKey(ctx, apiKeyString)
			if err != nil {
				return nil, err
			}
			newCtx := context.WithValue(ctx, userIDKey, userID)
			return handler(newCtx, req)
		}

		// 2. If no token, check if the method is public
		if i.publicRpcMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		// 3. Not public and no token -> Fail
		return nil, status.Error(codes.Unauthenticated, "missing authorization token or api key")
	}
}

func (i *AuthInterceptor) VerifyAPIKey(ctx context.Context, key string) (string, error) {
	// Simple validation
	if !strings.HasPrefix(key, "sk-") {
		return "", status.Error(codes.Unauthenticated, "invalid api key format")
	}

	h := sha256.New()
	h.Write([]byte(key))
	keyHash := hex.EncodeToString(h.Sum(nil))

	apiKey, err := i.apiKeyRepo.GetByHash(ctx, keyHash)
	if err != nil {
		// Log internal error?
		return "", status.Errorf(codes.Unauthenticated, "invalid api key")
	}
	if apiKey == nil || !apiKey.IsActive {
		return "", status.Error(codes.Unauthenticated, "api key is inactive or invalid")
	}
	if apiKey.ExpiresAt.Valid && apiKey.ExpiresAt.Time.Before(time.Now()) {
		return "", status.Error(codes.Unauthenticated, "api key expired")
	}

	// Reject API keys belonging to banned users.
	if i.userRepo != nil {
		user, err := i.userRepo.GetByID(ctx, apiKey.UserID)
		if err != nil || user == nil {
			return "", status.Error(codes.Unauthenticated, "invalid api key")
		}
		if user.Status == "banned" {
			return "", status.Error(codes.PermissionDenied, "account banned")
		}
	}

	// Update last used asynchronously
	go func() {
		_ = i.apiKeyRepo.UpdateLastUsed(context.Background(), apiKey.ID)
	}()

	return apiKey.UserID, nil
}

// GetUserIDFromContext retrieves the user ID from the context.
func GetUserIDFromContext(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(userIDKey).(string)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "user not authenticated")
	}
	return userID, nil
}

// VerifyHTTPRequest validates the request credentials (Token or API Key) and returns the user ID.
func (i *AuthInterceptor) VerifyHTTPRequest(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 {
			scheme := strings.ToLower(parts[0])
			switch scheme {
			case "bearer":
				return i.VerifyToken(parts[1])
			case "apikey":
				return i.VerifyAPIKey(r.Context(), parts[1])
			}
		}
	}

	// Check X-API-Key header
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		return i.VerifyAPIKey(r.Context(), apiKey)
	}

	return "", status.Error(codes.Unauthenticated, "missing or invalid credentials")
}
