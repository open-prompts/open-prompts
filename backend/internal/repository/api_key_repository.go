package repository

import (
	"context"
	"database/sql"
	"fmt"

	"open-prompts/backend/internal/models"
)

type APIKeyRepository struct {
	db *sql.DB
}

func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Create(ctx context.Context, apiKey *models.APIKey) error {
	query := `
		INSERT INTO api_keys (user_id, name, key_hash, prefix, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, is_active
	`
	// Assuming expires_at is null if not set, or we can handle it
	var expiresAt interface{}
	if apiKey.ExpiresAt.Valid {
		expiresAt = apiKey.ExpiresAt.Time
	} else {
		expiresAt = nil
	}

	err := r.db.QueryRowContext(ctx, query, apiKey.UserID, apiKey.Name, apiKey.KeyHash, apiKey.Prefix, expiresAt).
		Scan(&apiKey.ID, &apiKey.CreatedAt, &apiKey.IsActive)
	if err != nil {
		return fmt.Errorf("failed to create api key: %w", err)
	}
	return nil
}

func (r *APIKeyRepository) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*models.APIKey, error) {
	query := `
		SELECT id, user_id, name, prefix, created_at, expires_at, last_used_at, is_active
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var keys []*models.APIKey
	for rows.Next() {
		k := &models.APIKey{}
		var expiresAt, lastUsedAt sql.NullTime
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.CreatedAt, &expiresAt, &lastUsedAt, &k.IsActive); err != nil {
			return nil, fmt.Errorf("failed to scan api key: %w", err)
		}
		k.ExpiresAt = expiresAt
		k.LastUsedAt = lastUsedAt
		keys = append(keys, k)
	}
	return keys, nil
}

func (r *APIKeyRepository) GetByHash(ctx context.Context, hash string) (*models.APIKey, error) {
	query := `
		SELECT id, user_id, name, prefix, created_at, expires_at, last_used_at, is_active
		FROM api_keys
		WHERE key_hash = $1
	`
	k := &models.APIKey{KeyHash: hash}
	var expiresAt, lastUsedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, hash).
		Scan(&k.ID, &k.UserID, &k.Name, &k.Prefix, &k.CreatedAt, &expiresAt, &lastUsedAt, &k.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Or specific error
		}
		return nil, fmt.Errorf("failed to get api key by hash: %w", err)
	}
	k.ExpiresAt = expiresAt
	k.LastUsedAt = lastUsedAt
	return k, nil
}

func (r *APIKeyRepository) Delete(ctx context.Context, id, userID string) error {
	query := `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`
	result, err := r.db.ExecContext(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete api key: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows // Or custom error "api key not found or access denied"
	}
	return nil
}

func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}
	return nil
}
