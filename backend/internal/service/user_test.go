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
"golang.org/x/crypto/bcrypt"
)

// MockUserRepository is a mock implementation of repository.UserRepository
type MockUserRepository struct {
mock.Mock
}

func (m *MockUserRepository) Insert(ctx context.Context, user *models.User) error {
args := m.Called(ctx, user)
return args.Error(0)
}

func (m *MockUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
args := m.Called(ctx, email)
if args.Get(0) == nil {
return nil, args.Error(1)
}
return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
args := m.Called(ctx, id)
if args.Get(0) == nil {
return nil, args.Error(1)
}
return args.Get(0).(*models.User), args.Error(1)
}

func (m *MockUserRepository) Update(ctx context.Context, user *models.User) error {
args := m.Called(ctx, user)
return args.Error(0)
}

// MockRedisStore is a mock implementation of RedisStore
type MockRedisStore struct {
mock.Mock
}

func (m *MockRedisStore) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
args := m.Called(ctx, key, value, expiration)
return args.Error(0)
}

func (m *MockRedisStore) Get(ctx context.Context, key string) (string, error) {
args := m.Called(ctx, key)
return args.String(0), args.Error(1)
}

func (m *MockRedisStore) Del(ctx context.Context, key string) error {
args := m.Called(ctx, key)
return args.Error(0)
}

// MockEmailService is a mock implementation of EmailService
type MockEmailService struct {
mock.Mock
}

func (m *MockEmailService) SendVerificationCode(to, code, lang string) error {
args := m.Called(to, code, lang)
return args.Error(0)
}

func TestRegister(t *testing.T) {
t.Run("Success", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.RegisterRequest{
Id:               "user_123",
Email:            "test@example.com",
Password:         "password123",
DisplayName:      "Test User",
VerificationCode: "123456",
}

mockRedis.On("Get", mock.Anything, "verify_email:test@example.com").Return("123456", nil)
mockRedis.On("Del", mock.Anything, "verify_email:test@example.com").Return(nil)

mockRepo.On("GetByID", mock.Anything, req.Id).Return(nil, repository.ErrUserNotFound)
mockRepo.On("GetByEmail", mock.Anything, req.Email).Return(nil, repository.ErrUserNotFound)
mockRepo.On("Insert", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
return u.ID == req.Id && u.Email == req.Email && u.DisplayName == req.DisplayName
})).Return(nil)

resp, err := svc.Register(context.Background(), req)

assert.NoError(t, err)
assert.NotNil(t, resp)
assert.Equal(t, req.Id, resp.Id)
assert.NotEmpty(t, resp.Token)
mockRepo.AssertExpectations(t)
mockRedis.AssertExpectations(t)
})

t.Run("DuplicateID", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.RegisterRequest{
Id:               "user_123",
Email:            "test@example.com",
Password:         "password123",
VerificationCode: "123456",
}

mockRedis.On("Get", mock.Anything, "verify_email:test@example.com").Return("123456", nil)
mockRedis.On("Del", mock.Anything, "verify_email:test@example.com").Return(nil)

mockRepo.On("GetByID", mock.Anything, req.Id).Return(&models.User{}, nil)

resp, err := svc.Register(context.Background(), req)

assert.Error(t, err)
assert.Nil(t, resp)
assert.Contains(t, err.Error(), "id already exists")
})

t.Run("InvalidID", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.RegisterRequest{
Id:               "user-123", // Invalid character '-'
Email:            "test@example.com",
Password:         "password123",
VerificationCode: "123456",
}

mockRedis.On("Get", mock.Anything, "verify_email:test@example.com").Return("123456", nil)
mockRedis.On("Del", mock.Anything, "verify_email:test@example.com").Return(nil)

resp, err := svc.Register(context.Background(), req)

assert.Error(t, err)
assert.Nil(t, resp)
assert.Contains(t, err.Error(), "alphanumeric characters and underscores")
})

t.Run("InvalidVerificationCode", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.RegisterRequest{
Id:               "user_123",
Email:            "test@example.com",
Password:         "password123",
VerificationCode: "wrong_code",
}

mockRedis.On("Get", mock.Anything, "verify_email:test@example.com").Return("123456", nil)

resp, err := svc.Register(context.Background(), req)

assert.Error(t, err)
assert.Nil(t, resp)
assert.Contains(t, err.Error(), "invalid verification code")
})
}

func TestLogin(t *testing.T) {
password := "password123"
hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

user := &models.User{
ID:           "user_123",
Email:        "test@example.com",
PasswordHash: string(hash),
DisplayName:  "Test User",
}

t.Run("SuccessByEmail", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.LoginRequest{
Email:    "test@example.com",
Password: password,
}

mockRepo.On("GetByEmail", mock.Anything, req.Email).Return(user, nil)

resp, err := svc.Login(context.Background(), req)

assert.NoError(t, err)
assert.NotNil(t, resp)
assert.Equal(t, user.ID, resp.Id)
assert.NotEmpty(t, resp.Token)
})

t.Run("SuccessByID", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.LoginRequest{
Email:    "user_123", // Using ID in Email field as identifier
Password: password,
}

mockRepo.On("GetByEmail", mock.Anything, req.Email).Return(nil, repository.ErrUserNotFound)
mockRepo.On("GetByID", mock.Anything, req.Email).Return(user, nil)

resp, err := svc.Login(context.Background(), req)

assert.NoError(t, err)
assert.NotNil(t, resp)
assert.Equal(t, user.ID, resp.Id)
})

t.Run("InvalidPassword", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.LoginRequest{
Email:    "test@example.com",
Password: "wrongpassword",
}

mockRepo.On("GetByEmail", mock.Anything, req.Email).Return(user, nil)

resp, err := svc.Login(context.Background(), req)

assert.Error(t, err)
assert.Nil(t, resp)
assert.Contains(t, err.Error(), "invalid credentials")
})

t.Run("UserNotFound", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")
req := &pb.LoginRequest{
Email:    "unknown@example.com",
Password: "password",
}

mockRepo.On("GetByEmail", mock.Anything, req.Email).Return(nil, repository.ErrUserNotFound)
mockRepo.On("GetByID", mock.Anything, req.Email).Return(nil, repository.ErrUserNotFound)

resp, err := svc.Login(context.Background(), req)

assert.Error(t, err)
assert.Nil(t, resp)
assert.Contains(t, err.Error(), "invalid credentials")
})
}

func TestUpdateProfile(t *testing.T) {
t.Run("Success", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")

userID := "user_123"
existingUser := &models.User{
ID:           userID,
DisplayName:  "Old Name",
PasswordHash: "hashed_password",
}

req := &pb.UpdateProfileRequest{
Id:          userID,
DisplayName: "New Name",
Avatar:      "avatar_data",
}

mockRepo.On("GetByID", mock.Anything, userID).Return(existingUser, nil)
mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
return u.ID == userID && u.DisplayName == "New Name" && u.Avatar == "avatar_data"
})).Return(nil)

resp, err := svc.UpdateProfile(context.Background(), req)

assert.NoError(t, err)
assert.Equal(t, "New Name", resp.DisplayName)
assert.Equal(t, "avatar_data", resp.Avatar)
mockRepo.AssertExpectations(t)
})
}

// Added test for SendVerificationCode
func TestSendVerificationCode(t *testing.T) {
t.Run("Success_En", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")

req := &pb.SendVerificationCodeRequest{
Email:    "real_user@domain.com",
Language: "en",
}

// Mock redis set - args: key, value, expiration
mockRedis.On("Set", mock.Anything, "verify_email:real_user@domain.com", mock.Anything, 5*time.Minute).Return(nil)

// Mock email send
mockEmail.On("SendVerificationCode", req.Email, mock.Anything, "en").Return(nil)

resp, err := svc.SendVerificationCode(context.Background(), req)
assert.NoError(t, err)
assert.True(t, resp.Success)
mockRedis.AssertExpectations(t)
mockEmail.AssertExpectations(t)
})

t.Run("Success_TestUser", func(t *testing.T) {
mockRepo := new(MockUserRepository)
mockRedis := new(MockRedisStore)
mockEmail := new(MockEmailService)
svc := NewUserService(mockRepo, mockRedis, mockEmail, "secret")

req := &pb.SendVerificationCodeRequest{
Email:    "test@example.com",
}

// Mock redis set for test code - value should be 123456
mockRedis.On("Set", mock.Anything, "verify_email:test@example.com", "123456", 5*time.Minute).Return(nil)

// Email should NOT be called for test user

resp, err := svc.SendVerificationCode(context.Background(), req)
assert.NoError(t, err)
assert.True(t, resp.Success)
mockRedis.AssertExpectations(t)
// No assert on mockEmail since we expect no calls
})
}
