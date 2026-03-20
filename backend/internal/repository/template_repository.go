package repository

import (
	"context"
	"database/sql"
	"fmt"

	"open-prompts/backend/internal/models"

	"github.com/lib/pq"
)

// TemplateRepository defines the interface for template data access.
type TemplateRepository interface {
	List(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*models.Template, error)
	Create(ctx context.Context, template *models.Template) error
	Update(ctx context.Context, template *models.Template) error
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string, currentUserID string) (*models.Template, error)
	ListCategories(ctx context.Context, filters map[string]interface{}) ([]*models.CategoryStat, error)
	ListTags(ctx context.Context, filters map[string]interface{}) ([]*models.TagStat, error)
	ToggleLike(ctx context.Context, userID, templateID string) (bool, int32, error)
	ToggleFavorite(ctx context.Context, userID, templateID string) (bool, int32, error)
}

// templateRepository implements TemplateRepository.
type templateRepository struct {
	db *sql.DB
}

// NewTemplateRepository creates a new instance of TemplateRepository.
func NewTemplateRepository(db *sql.DB) TemplateRepository {
	return &templateRepository{db: db}
}

// Create inserts a new template into the database.
func (r *templateRepository) Create(ctx context.Context, t *models.Template) error {
	query := `
		INSERT INTO templates (
			owner_id, title, description, visibility, type, tags, category, language, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		t.OwnerID, t.Title, t.Description, t.Visibility, t.Type, pq.Array(t.Tags), t.Category, t.Language, t.CreatedAt, t.UpdatedAt,
	).Scan(&t.ID)
	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}
	return nil
}

// Update updates an existing template in the database.
func (r *templateRepository) Update(ctx context.Context, t *models.Template) error {
	query := `
		UPDATE templates
		SET title = $1, description = $2, visibility = $3, tags = $4, category = $5, language = $6, updated_at = $7
		WHERE id = $8
	`
	_, err := r.db.ExecContext(ctx, query,
		t.Title, t.Description, t.Visibility, pq.Array(t.Tags), t.Category, t.Language, t.UpdatedAt, t.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}
	return nil
}

// Delete removes a template from the database.
func (r *templateRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM templates WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}
	return nil
}

// Get retrieves a template by ID.
func (r *templateRepository) Get(ctx context.Context, id string, currentUserID string) (*models.Template, error) {
	query := `
		SELECT
			t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags, t.category, t.language,
			t.like_count, t.favorite_count, t.created_at, t.updated_at,
			CASE WHEN tl.user_id IS NOT NULL THEN true ELSE false END as is_liked,
			CASE WHEN tf.user_id IS NOT NULL THEN true ELSE false END as is_favorited
		FROM templates t
		LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $2
		LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $2
		WHERE t.id = $1
	`
	var t models.Template
	err := r.db.QueryRowContext(ctx, query, id, currentUserID).Scan(
		&t.ID, &t.OwnerID, &t.Title, &t.Description, &t.Visibility, &t.Type,
		&t.Tags, &t.Category, &t.Language, &t.LikeCount, &t.FavoriteCount, &t.CreatedAt, &t.UpdatedAt,
		&t.IsLiked, &t.IsFavorited,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("template not found")
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	return &t, nil
}

// List retrieves a list of templates based on filters and pagination.
func (r *templateRepository) List(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*models.Template, error) {
	currentUserID := ""
	if val, ok := filters["current_user_id"]; ok {
		currentUserID = val.(string)
	}

	query := `
		SELECT
			t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags, t.category, t.language,
			t.like_count, t.favorite_count, t.created_at, t.updated_at,
			CASE WHEN tl.user_id IS NOT NULL THEN true ELSE false END as is_liked,
			CASE WHEN tf.user_id IS NOT NULL THEN true ELSE false END as is_favorited
		FROM templates t
		LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $1
		LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $1
		WHERE 1=1
	`
	args := []interface{}{currentUserID}
	argID := 2

	if val, ok := filters["visibility"]; ok && val != "" {
		query += fmt.Sprintf(" AND t.visibility = $%d", argID)
		args = append(args, val)
		argID++
	}
	if val, ok := filters["owner_id"]; ok && val != "" {
		query += fmt.Sprintf(" AND t.owner_id = $%d", argID)
		args = append(args, val)
		argID++
	}
	if val, ok := filters["category"]; ok && val != "" {
		query += fmt.Sprintf(" AND t.category = $%d", argID)
		args = append(args, val)
		argID++
	}
	if val, ok := filters["language"]; ok && val != "" {
		query += fmt.Sprintf(" AND t.language = $%d", argID)
		args = append(args, val)
		argID++
	}
	if val, ok := filters["tags"]; ok {
		tags := val.([]string)
		if len(tags) > 0 {
			query += fmt.Sprintf(" AND t.tags @> $%d", argID)
			args = append(args, pq.Array(tags))
			argID++
		}
	}
	if val, ok := filters["my_likes"]; ok && val.(bool) {
		query += " AND tl.user_id IS NOT NULL"
	}
	if val, ok := filters["my_favorites"]; ok && val.(bool) {
		query += " AND tf.user_id IS NOT NULL"
	}

	query += fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", argID, argID+1)
	args = append(args, limit, offset)

	// fmt.Printf("List Query: %s, Args: %v\n", query, args)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query templates: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var templates []*models.Template
	for rows.Next() {
		var t models.Template
		if err := rows.Scan(
			&t.ID, &t.OwnerID, &t.Title, &t.Description, &t.Visibility, &t.Type,
			&t.Tags, &t.Category, &t.Language, &t.LikeCount, &t.FavoriteCount, &t.CreatedAt, &t.UpdatedAt,
			&t.IsLiked, &t.IsFavorited,
		); err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		templates = append(templates, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return templates, nil
}

// ListCategories retrieves all categories and their template counts.
func (r *templateRepository) ListCategories(ctx context.Context, filters map[string]interface{}) ([]*models.CategoryStat, error) {
	query := `
		SELECT category, COUNT(*) as count
		FROM templates
		WHERE category IS NOT NULL AND category != ''
	`
	var args []interface{}
	argID := 1

	if val, ok := filters["visibility"]; ok && val != "" {
		query += fmt.Sprintf(" AND visibility = $%d", argID)
		args = append(args, val)
		argID++
	}
	if val, ok := filters["owner_id"]; ok && val != "" {
		query += fmt.Sprintf(" AND owner_id = $%d", argID)
		args = append(args, val)
		argID++
	}
	if val, ok := filters["language"]; ok && val != "" {
		query += fmt.Sprintf(" AND language = $%d", argID)
		args = append(args, val)
	}

	query += `
		GROUP BY category
		ORDER BY count DESC
	`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []*models.CategoryStat
	for rows.Next() {
		var s models.CategoryStat
		if err := rows.Scan(&s.Name, &s.Count); err != nil {
			return nil, fmt.Errorf("failed to scan category stat: %w", err)
		}
		stats = append(stats, &s)
	}
	return stats, nil
}

// ListTags retrieves all tags and their template counts.
func (r *templateRepository) ListTags(ctx context.Context, filters map[string]interface{}) ([]*models.TagStat, error) {
	var args []interface{}
	argID := 1

	// Note: unnesting happens in select, filtering needs to happen on the row before or after?
	// If we filter templates first, then unnest, we count tags of visible templates.
	// Correct approach:
	// SELECT t.tag, COUNT(*) FROM (SELECT unnest(tags) as tag FROM templates WHERE ...) t GROUP BY t.tag

	// Re-writing query for safety with filters
	whereClause := ""
	if val, ok := filters["visibility"]; ok && val != "" {
		whereClause += fmt.Sprintf(" AND visibility = $%d", argID)
		args = append(args, val)
		argID++
	}
	if val, ok := filters["owner_id"]; ok && val != "" {
		whereClause += fmt.Sprintf(" AND owner_id = $%d", argID)
		args = append(args, val)
		argID++ // Ensure argID is incremented
	}
	if val, ok := filters["language"]; ok && val != "" {
		whereClause += fmt.Sprintf(" AND language = $%d", argID)
		args = append(args, val)
	}

	query := fmt.Sprintf(`
		SELECT tag, COUNT(*) as count
		FROM (
			SELECT unnest(tags) as tag
			FROM templates
			WHERE 1=1 %s
		) as t
		GROUP BY tag
		ORDER BY count DESC
	`, whereClause)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []*models.TagStat
	for rows.Next() {
		var s models.TagStat
		if err := rows.Scan(&s.Name, &s.Count); err != nil {
			return nil, fmt.Errorf("failed to scan tag stat: %w", err)
		}
		stats = append(stats, &s)
	}
	return stats, nil
}

// ToggleLike toggles the like status of a template for a user.
func (r *templateRepository) ToggleLike(ctx context.Context, userID, templateID string) (bool, int32, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM template_likes WHERE user_id=$1 AND template_id=$2)", userID, templateID).Scan(&exists)
	if err != nil {
		return false, 0, err
	}

	if exists {
		_, err = tx.ExecContext(ctx, "DELETE FROM template_likes WHERE user_id=$1 AND template_id=$2", userID, templateID)
	} else {
		_, err = tx.ExecContext(ctx, "INSERT INTO template_likes (user_id, template_id) VALUES ($1, $2)", userID, templateID)
	}
	if err != nil {
		return false, 0, err
	}

	var count int32
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM template_likes WHERE template_id=$1", templateID).Scan(&count)
	if err != nil {
		return false, 0, err
	}

	_, err = tx.ExecContext(ctx, "UPDATE templates SET like_count=$1 WHERE id=$2", count, templateID)
	if err != nil {
		return false, 0, err
	}

	if err := tx.Commit(); err != nil {
		return false, 0, err
	}

	return !exists, count, nil
}

// ToggleFavorite toggles the favorite status of a template for a user.
func (r *templateRepository) ToggleFavorite(ctx context.Context, userID, templateID string) (bool, int32, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM template_favorites WHERE user_id=$1 AND template_id=$2)", userID, templateID).Scan(&exists)
	if err != nil {
		return false, 0, err
	}

	if exists {
		_, err = tx.ExecContext(ctx, "DELETE FROM template_favorites WHERE user_id=$1 AND template_id=$2", userID, templateID)
	} else {
		_, err = tx.ExecContext(ctx, "INSERT INTO template_favorites (user_id, template_id) VALUES ($1, $2)", userID, templateID)
	}
	if err != nil {
		return false, 0, err
	}

	var count int32
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM template_favorites WHERE template_id=$1", templateID).Scan(&count)
	if err != nil {
		return false, 0, err
	}

	_, err = tx.ExecContext(ctx, "UPDATE templates SET favorite_count=$1 WHERE id=$2", count, templateID)
	if err != nil {
		return false, 0, err
	}

	if err := tx.Commit(); err != nil {
		return false, 0, err
	}

	return !exists, count, nil
}
