package repository

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/zap"

	"awsome-prompt/backend/internal/models"
)

// TemplateVersionRepository defines the interface for template version data access.
type TemplateVersionRepository interface {
	Create(ctx context.Context, version *models.TemplateVersion) error
	GetLatest(ctx context.Context, templateID string) (*models.TemplateVersion, error)
	Get(ctx context.Context, id int32) (*models.TemplateVersion, error)
	List(ctx context.Context, limit, offset int, templateID string) ([]*models.TemplateVersion, error)
}

type templateVersionRepository struct {
	db *sql.DB
}

// NewTemplateVersionRepository creates a new instance of TemplateVersionRepository.
func NewTemplateVersionRepository(db *sql.DB) TemplateVersionRepository {
	return &templateVersionRepository{db: db}
}

// Create inserts a new template version into the database.
func (r *templateVersionRepository) Create(ctx context.Context, v *models.TemplateVersion) error {
	zap.S().Infof("TemplateVersionRepository.Create: templateID=%s version=%s", v.TemplateID, v.Version)
	query := `
		INSERT INTO template_versions (template_id, version, content, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		v.TemplateID, v.Version, v.Content, v.CreatedAt,
	).Scan(&v.ID)

	if err != nil {
		return fmt.Errorf("failed to create template version: %w", err)
	}
	return nil
}

// GetLatest retrieves the latest version of a template.
func (r *templateVersionRepository) GetLatest(ctx context.Context, templateID string) (*models.TemplateVersion, error) {
	zap.S().Infof("TemplateVersionRepository.GetLatest: templateID=%s", templateID)
	query := `
		SELECT id, template_id, version, content, created_at
		FROM template_versions
		WHERE template_id = $1
		ORDER BY version DESC
		LIMIT 1
	`
	var v models.TemplateVersion
	err := r.db.QueryRowContext(ctx, query, templateID).Scan(
		&v.ID, &v.TemplateID, &v.Version, &v.Content, &v.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest version: %w", err)
	}
	return &v, nil
}

// List retrieves all versions for a template.
func (r *templateVersionRepository) List(ctx context.Context, limit, offset int, templateID string) ([]*models.TemplateVersion, error) {
	zap.S().Infof("TemplateVersionRepository.List: templateID=%s limit=%d offset=%d", templateID, limit, offset)
	query := `
		SELECT id, template_id, version, content, created_at
		FROM template_versions
		WHERE template_id = $1
		ORDER BY version DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, templateID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list template versions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var versions []*models.TemplateVersion
	for rows.Next() {
		var v models.TemplateVersion
		if err := rows.Scan(
			&v.ID, &v.TemplateID, &v.Version, &v.Content, &v.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan template version: %w", err)
		}
		versions = append(versions, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return versions, nil
}

// Get retrieves a specific version by its ID.
func (r *templateVersionRepository) Get(ctx context.Context, id int32) (*models.TemplateVersion, error) {
	query := `
SELECT id, template_id, version, content, created_at
FROM template_versions
WHERE id = $1
`
	var v models.TemplateVersion
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&v.ID, &v.TemplateID, &v.Version, &v.Content, &v.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get template version: %w", err)
	}
	return &v, nil
}
