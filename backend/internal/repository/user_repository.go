package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.uber.org/zap"

	"open-prompts/backend/internal/models"
)

var (
	ErrUserNotFound = errors.New("user not found")
)

type UserRepository interface {
	Insert(ctx context.Context, user *models.User) error
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByID(ctx context.Context, id string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	UpdateRoleAndStatus(ctx context.Context, userID, role, status string) error
}

type userRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Insert(ctx context.Context, user *models.User) error {
	zap.S().Infof("UserRepository.Insert: email=%s", user.Email)
	query := `
INSERT INTO users (id, email, mobile, password_hash, display_name, avatar, role, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING created_at, updated_at`

	role := user.Role
	if role == "" {
		role = "user"
	}
	status := user.Status
	if status == "" {
		status = "active"
	}

	args := []interface{}{
		user.ID,
		user.Email,
		user.Mobile,
		user.PasswordHash,
		user.DisplayName,
		user.Avatar,
		role,
		status,
		time.Now(),
		time.Now(),
	}

	return r.db.QueryRowContext(ctx, query, args...).Scan(&user.CreatedAt, &user.UpdatedAt)
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	zap.S().Infof("UserRepository.GetByEmail: email=%s", email)
	query := `
SELECT id, email, mobile, password_hash, display_name, COALESCE(avatar, ''), role, status,
       approved_template_count, review_gated_remaining, show_warned_content,
       COALESCE(bio, ''), follower_count, following_count, created_at, updated_at
FROM users
WHERE email = $1`

	var user models.User
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Mobile,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Avatar,
		&user.Role,
		&user.Status,
		&user.ApprovedTemplateCount,
		&user.ReviewGatedRemaining,
		&user.ShowWarnedContent,
		&user.Bio,
		&user.FollowerCount,
		&user.FollowingCount,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	zap.S().Infof("UserRepository.GetByID: id=%s", id)
	query := `
SELECT id, email, mobile, password_hash, display_name, COALESCE(avatar, ''), role, status,
       approved_template_count, review_gated_remaining, show_warned_content,
       COALESCE(bio, ''), follower_count, following_count, created_at, updated_at
FROM users
WHERE id = $1`

	var user models.User
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.Mobile,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Avatar,
		&user.Role,
		&user.Status,
		&user.ApprovedTemplateCount,
		&user.ReviewGatedRemaining,
		&user.ShowWarnedContent,
		&user.Bio,
		&user.FollowerCount,
		&user.FollowingCount,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &user, nil
}

func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	zap.S().Infof("UserRepository.Update: id=%s", user.ID)
	query := `
UPDATE users
SET email = $2, mobile = $3, password_hash = $4, display_name = $5, avatar = $6,
    show_warned_content = $7, bio = $8, updated_at = $9
WHERE id = $1`

	args := []interface{}{
		user.ID,
		user.Email,
		user.Mobile,
		user.PasswordHash,
		user.DisplayName,
		user.Avatar,
		user.ShowWarnedContent,
		user.Bio,
		time.Now(),
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

// UpdateRoleAndStatus sets the role and status of a user (admin actions).
func (r *userRepository) UpdateRoleAndStatus(ctx context.Context, userID, role, status string) error {
	zap.S().Infof("UserRepository.UpdateRoleAndStatus: id=%s role=%s status=%s", userID, role, status)
	query := `
UPDATE users
SET role = $2, status = $3, updated_at = NOW()
WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, userID, role, status)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}
