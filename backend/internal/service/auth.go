package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
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
	publicRpcMethods map[string]bool
}

type contextKey string

const userIDKey contextKey = "user_id"

// ContextWithUserID adds the user ID to the context.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
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
			// For testing reflection
			"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": true,
		},
	}
}

// VerifyToken validates the token string and returns the user ID.
func (i *AuthInterceptor) VerifyToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return i.jwtSecret, nil
	})

	if err != nil {
		return "", status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, ok := claims["sub"].(string)
		if !ok {
			return "", status.Error(codes.Unauthenticated, "invalid token payload: missing sub")
		}
		return userID, nil
	}

	return "", status.Error(codes.Unauthenticated, "invalid token")
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
			userID, err := i.VerifyToken(tokenString)
			if err != nil {
				return nil, err // Fail if token provided but invalid
			}
			// Inject User ID into Context
			newCtx := context.WithValue(ctx, userIDKey, userID)
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
