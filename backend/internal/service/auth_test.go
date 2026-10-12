package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"open-prompts/backend/internal/models"
	"open-prompts/backend/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newTestInterceptor builds an AuthInterceptor with mock dependencies.
func newTestInterceptor() (*AuthInterceptor, *MockUserRepository, *MockRedisStore) {
	mockUser := new(MockUserRepository)
	mockRedis := new(MockRedisStore)
	i := NewAuthInterceptor("secret", nil)
	i.SetUserRepository(mockUser)
	i.SetRedis(mockRedis)
	return i, mockUser, mockRedis
}

// makeJWT signs a token with the given claims for the test secret.
func makeJWT(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte("secret"))
	return s
}

func TestVerifyTokenWithClaims(t *testing.T) {
	t.Run("ValidTokenWithRoleAndScope", func(t *testing.T) {
		i, _, _ := newTestInterceptor()
		token := makeJWT(jwt.MapClaims{
			"sub":   "user_1",
			"role":  "admin",
			"scope": "admin",
			"iat":   float64(time.Now().Unix()),
			"exp":   float64(time.Now().Add(time.Hour).Unix()),
		})
		claims, err := i.VerifyTokenWithClaims(token)
		assert.NoError(t, err)
		assert.Equal(t, "user_1", claims.UserID)
		assert.Equal(t, "admin", claims.Role)
		assert.Equal(t, "admin", claims.Scope)
	})

	t.Run("LegacyTokenWithoutRole", func(t *testing.T) {
		i, _, _ := newTestInterceptor()
		token := makeJWT(jwt.MapClaims{
			"sub": "user_1",
			"exp": float64(time.Now().Add(time.Hour).Unix()),
		})
		claims, err := i.VerifyTokenWithClaims(token)
		assert.NoError(t, err)
		assert.Equal(t, "user_1", claims.UserID)
		assert.Equal(t, "", claims.Role)
	})

	t.Run("ExpiredToken", func(t *testing.T) {
		i, _, _ := newTestInterceptor()
		token := makeJWT(jwt.MapClaims{
			"sub": "user_1",
			"exp": float64(time.Now().Add(-time.Hour).Unix()),
		})
		_, err := i.VerifyTokenWithClaims(token)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("GarbageToken", func(t *testing.T) {
		i, _, _ := newTestInterceptor()
		_, err := i.VerifyTokenWithClaims("not-a-token")
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("WrongSecret", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user_1"})
		s, _ := token.SignedString([]byte("other-secret"))
		i, _, _ := newTestInterceptor()
		_, err := i.VerifyTokenWithClaims(s)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("MissingSub", func(t *testing.T) {
		i, _, _ := newTestInterceptor()
		token := makeJWT(jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())})
		_, err := i.VerifyTokenWithClaims(token)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

func TestCheckBanAndRevocation(t *testing.T) {
	t.Run("BannedInRedis", func(t *testing.T) {
		i, _, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, "user_banned:user_1").Return("1", nil)
		err := i.checkBanAndRevocation(context.Background(), &TokenClaims{UserID: "user_1"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("TokenRevoked", func(t *testing.T) {
		i, _, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, "user_banned:user_1").Return("", errors.New("not found"))
		mockRedis.On("Get", mock.Anything, "token_revoked_before:user_1").Return("9999999999", nil)
		err := i.checkBanAndRevocation(context.Background(), &TokenClaims{UserID: "user_1", Iat: 1000})
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})

	t.Run("TokenIssuedAfterRevocation", func(t *testing.T) {
		i, _, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, "user_banned:user_1").Return("", errors.New("not found"))
		mockRedis.On("Get", mock.Anything, "token_revoked_before:user_1").Return("1000", nil)
		err := i.checkBanAndRevocation(context.Background(), &TokenClaims{UserID: "user_1", Iat: 2000})
		assert.NoError(t, err)
	})

	t.Run("RedisErrorFallsBackToDB", func(t *testing.T) {
		i, mockUser, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, mock.Anything).Return("", errors.New("redis down"))
		mockUser.On("GetByID", mock.Anything, "user_1").Return(&models.User{ID: "user_1", Status: "active"}, nil)
		err := i.checkBanAndRevocation(context.Background(), &TokenClaims{UserID: "user_1"})
		assert.NoError(t, err)
	})

	t.Run("DBFallbackBanned", func(t *testing.T) {
		i, mockUser, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, mock.Anything).Return("", errors.New("redis down"))
		mockUser.On("GetByID", mock.Anything, "user_1").Return(&models.User{ID: "user_1", Status: "banned"}, nil)
		err := i.checkBanAndRevocation(context.Background(), &TokenClaims{UserID: "user_1"})
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

func TestVerifyAnyToken(t *testing.T) {
	t.Run("JWTBannedUser", func(t *testing.T) {
		i, _, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, "user_banned:user_1").Return("1", nil)
		token := makeJWT(jwt.MapClaims{
			"sub": "user_1",
			"exp": float64(time.Now().Add(time.Hour).Unix()),
		})
		_, err := i.VerifyAnyToken(context.Background(), token)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("JWTActiveUser", func(t *testing.T) {
		i, mockUser, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, mock.Anything).Return("", errors.New("not found"))
		mockUser.On("GetByID", mock.Anything, "user_1").Return(&models.User{ID: "user_1", Status: "active"}, nil)
		token := makeJWT(jwt.MapClaims{
			"sub": "user_1",
			"exp": float64(time.Now().Add(time.Hour).Unix()),
		})
		userID, err := i.VerifyAnyToken(context.Background(), token)
		assert.NoError(t, err)
		assert.Equal(t, "user_1", userID)
	})
}

func TestClientIPFromContext(t *testing.T) {
	t.Run("FromMetadata", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-client-ip", "1.2.3.4"))
		assert.Equal(t, "1.2.3.4", clientIPFromContext(ctx))
	})

	t.Run("Missing", func(t *testing.T) {
		assert.Equal(t, "unknown", clientIPFromContext(context.Background()))
	})
}

func TestVerifyHTTPRequest(t *testing.T) {
	t.Run("BearerToken", func(t *testing.T) {
		i, mockUser, mockRedis := newTestInterceptor()
		mockRedis.On("Get", mock.Anything, mock.Anything).Return("", errors.New("not found"))
		mockUser.On("GetByID", mock.Anything, "user_1").Return(&models.User{ID: "user_1", Status: "active"}, nil)
		token := makeJWT(jwt.MapClaims{
			"sub": "user_1",
			"exp": float64(time.Now().Add(time.Hour).Unix()),
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		userID, err := i.VerifyHTTPRequest(req)
		assert.NoError(t, err)
		assert.Equal(t, "user_1", userID)
	})

	t.Run("MissingCredentials", func(t *testing.T) {
		i, _, _ := newTestInterceptor()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		_, err := i.VerifyHTTPRequest(req)
		assert.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

func TestContextRoleScope(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", GetUserRoleFromContext(ctx))
	assert.Equal(t, "", GetUserScopeFromContext(ctx))

	ctx = ContextWithUserRole(ctx, "admin")
	ctx = ContextWithUserScope(ctx, "admin")
	assert.Equal(t, "admin", GetUserRoleFromContext(ctx))
	assert.Equal(t, "admin", GetUserScopeFromContext(ctx))
}

func TestPublicRpcMethodsIncludeAdminLogin(t *testing.T) {
	i, _, _ := newTestInterceptor()
	assert.True(t, i.publicRpcMethods["/v1.AdminService/AdminLogin"])
	assert.True(t, i.publicRpcMethods["/v1.UserService/Login"])
}

func TestDBFallbackWithoutRedis(t *testing.T) {
	// Interceptor with no Redis wired: DB fallback path.
	i := NewAuthInterceptor("secret", nil)
	mockUser := new(MockUserRepository)
	i.SetUserRepository(mockUser)
	mockUser.On("GetByID", mock.Anything, "user_1").Return(&models.User{ID: "user_1", Status: "banned"}, nil)
	err := i.checkBanAndRevocation(context.Background(), &TokenClaims{UserID: "user_1"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestDBFallbackUserNotFound(t *testing.T) {
	i := NewAuthInterceptor("secret", nil)
	mockUser := new(MockUserRepository)
	i.SetUserRepository(mockUser)
	mockUser.On("GetByID", mock.Anything, "user_1").Return(nil, repository.ErrUserNotFound)
	err := i.checkBanAndRevocation(context.Background(), &TokenClaims{UserID: "user_1"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
