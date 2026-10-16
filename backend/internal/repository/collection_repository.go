package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"open-prompts/backend/internal/models"
)

// CollectionRepository is the data-access interface for template collections.
type CollectionRepository interface {
	// Create inserts a new collection.
	Create(ctx context.Context, c *models.Collection) error
	// Update sets name/description/visibility/updated_at.
	Update(ctx context.Context, c *models.Collection) error
	// SoftDelete marks a collection status='deleted' (updated_at=NOW()).
	SoftDelete(ctx context.Context, id string) error
	// GetByID loads a collection by id (any status; the service filters).
	GetByID(ctx context.Context, id string) (*models.Collection, error)
	// CountActiveByOwner returns the caller's active collection count.
	CountActiveByOwner(ctx context.Context, ownerID string) (int32, error)
	// ListByOwner returns the caller's active collections, newest first.
	ListByOwner(ctx context.Context, ownerID string) ([]*models.Collection, error)
	// ListPublic returns a page of public active collections, sorted.
	ListPublic(ctx context.Context, offset, limit int32, sort string) ([]*models.Collection, error)
	// CountPublic returns the number of public active collections.
	CountPublic(ctx context.Context) (int32, error)
	// AddMember inserts a member row (idempotent via ON CONFLICT DO NOTHING).
	AddMember(ctx context.Context, collectionID, templateID string, position int32) error
	// RemoveMember deletes a member row (missing row is a no-op).
	RemoveMember(ctx context.Context, collectionID, templateID string) error
	// ReorderMembers applies a full ordered template-id list in one tx.
	ReorderMembers(ctx context.Context, collectionID string, templateIDs []string) error
	// CountMembers returns the number of members in a collection.
	CountMembers(ctx context.Context, collectionID string) (int32, error)
	// MaxMemberPosition returns the highest position in a collection (-1 when empty).
	MaxMemberPosition(ctx context.Context, collectionID string) (int32, error)
	// GetMemberPosition returns a member's position, or -1 when not present.
	GetMemberPosition(ctx context.Context, collectionID, templateID string) (int32, error)
	// ListMembers returns a collection's members. ownerView=true returns all
	// members (flagged is_public); otherwise only public templates.
	ListMembers(ctx context.Context, collectionID string, ownerView bool) ([]*models.CollectionMember, error)
	// ListContainingTemplate returns the caller's active collections that
	// contain a given template (caller-scoped).
	ListContainingTemplate(ctx context.Context, ownerID, templateID string) ([]*models.Collection, error)
}

type collectionRepository struct {
	db *sql.DB
}

// NewCollectionRepository creates a new CollectionRepository.
func NewCollectionRepository(db *sql.DB) CollectionRepository {
	return &collectionRepository{db: db}
}

const collectionColumns = `id, owner_id, name, description, visibility, status, created_at, updated_at`

func scanCollection(row interface{ Scan(...any) error }) (*models.Collection, error) {
	var c models.Collection
	if err := row.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Description, &c.Visibility,
		&c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// Create inserts a new collection.
func (r *collectionRepository) Create(ctx context.Context, c *models.Collection) error {
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO collections (owner_id, name, description, visibility, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id, created_at, updated_at`,
		c.OwnerID, c.Name, c.Description, c.Visibility,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	return nil
}

// Update sets name/description/visibility/updated_at.
func (r *collectionRepository) Update(ctx context.Context, c *models.Collection) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE collections
		SET name = $2, description = $3, visibility = $4, updated_at = NOW()
		WHERE id = $1`,
		c.ID, c.Name, c.Description, c.Visibility)
	if err != nil {
		return fmt.Errorf("failed to update collection: %w", err)
	}
	return nil
}

// SoftDelete marks a collection status='deleted'.
func (r *collectionRepository) SoftDelete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE collections SET status = 'deleted', updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to soft-delete collection: %w", err)
	}
	return nil
}

// GetByID loads a collection by id (any status).
func (r *collectionRepository) GetByID(ctx context.Context, id string) (*models.Collection, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+collectionColumns+` FROM collections WHERE id = $1`, id)
	c, err := scanCollection(row)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}
	return c, nil
}

// CountActiveByOwner returns the caller's active collection count.
func (r *collectionRepository) CountActiveByOwner(ctx context.Context, ownerID string) (int32, error) {
	var count int32
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collections WHERE owner_id = $1 AND status = 'active'`, ownerID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count collections: %w", err)
	}
	return count, nil
}

// ListByOwner returns the caller's active collections, newest first.
func (r *collectionRepository) ListByOwner(ctx context.Context, ownerID string) ([]*models.Collection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.`+collectionColumns+`,
		       (SELECT COUNT(*) FROM collection_templates ct WHERE ct.collection_id = c.id) AS template_count
		FROM collections c
		WHERE c.owner_id = $1 AND c.status = 'active'
		ORDER BY c.created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cols []*models.Collection
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Description, &c.Visibility,
			&c.Status, &c.CreatedAt, &c.UpdatedAt, &c.TemplateCount); err != nil {
			return nil, fmt.Errorf("failed to scan collection: %w", err)
		}
		cols = append(cols, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate collections: %w", err)
	}
	return cols, nil
}

// ListPublic returns a page of public active collections, sorted.
func (r *collectionRepository) ListPublic(ctx context.Context, offset, limit int32, sort string) ([]*models.Collection, error) {
	orderBy := "c.created_at DESC"
	if sort == "most_templates" {
		orderBy = "template_count DESC, c.created_at DESC"
	}
	query := `
		SELECT c.` + collectionColumns + `,
		       (SELECT COUNT(*) FROM collection_templates ct WHERE ct.collection_id = c.id) AS template_count
		FROM collections c
		WHERE c.status = 'active' AND c.visibility = 'public'
		ORDER BY ` + orderBy + `
		LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list public collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cols []*models.Collection
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Description, &c.Visibility,
			&c.Status, &c.CreatedAt, &c.UpdatedAt, &c.TemplateCount); err != nil {
			return nil, fmt.Errorf("failed to scan public collection: %w", err)
		}
		cols = append(cols, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate public collections: %w", err)
	}
	return cols, nil
}

// CountPublic returns the number of public active collections.
func (r *collectionRepository) CountPublic(ctx context.Context) (int32, error) {
	var count int32
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collections WHERE status = 'active' AND visibility = 'public'`,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count public collections: %w", err)
	}
	return count, nil
}

// AddMember inserts a member row (idempotent).
func (r *collectionRepository) AddMember(ctx context.Context, collectionID, templateID string, position int32) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO collection_templates (collection_id, template_id, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (collection_id, template_id) DO NOTHING`,
		collectionID, templateID, position)
	if err != nil {
		return fmt.Errorf("failed to add collection member: %w", err)
	}
	return nil
}

// RemoveMember deletes a member row (missing row is a no-op).
func (r *collectionRepository) RemoveMember(ctx context.Context, collectionID, templateID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM collection_templates WHERE collection_id = $1 AND template_id = $2`,
		collectionID, templateID)
	if err != nil {
		return fmt.Errorf("failed to remove collection member: %w", err)
	}
	return nil
}

// ReorderMembers applies a full ordered template-id list in one tx, preserving
// each member's original added_at.
func (r *collectionRepository) ReorderMembers(ctx context.Context, collectionID string, templateIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin reorder tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Hold the original added_at values before deleting.
	hold := make(map[string]interface{}, len(templateIDs))
	rows, err := tx.QueryContext(ctx,
		`SELECT template_id, added_at FROM collection_templates WHERE collection_id = $1`, collectionID)
	if err != nil {
		return fmt.Errorf("failed to read members for reorder: %w", err)
	}
	for rows.Next() {
		var tid string
		var addedAt interface{}
		if err := rows.Scan(&tid, &addedAt); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan member for reorder: %w", err)
		}
		hold[tid] = addedAt
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate members for reorder: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM collection_templates WHERE collection_id = $1`, collectionID); err != nil {
		return fmt.Errorf("failed to clear members for reorder: %w", err)
	}

	// Re-insert with the original added_at (or NOW() for any new id).
	values := make([]string, 0, len(templateIDs))
	args := make([]interface{}, 0, len(templateIDs)*3+1)
	args = append(args, collectionID)
	for i, tid := range templateIDs {
		values = append(values, fmt.Sprintf("($1, $%d, $%d, $%d)", i*3+2, i*3+3, i*3+4))
		args = append(args, tid, i, hold[tid])
	}
	query := `INSERT INTO collection_templates (collection_id, template_id, position, added_at) VALUES ` +
		strings.Join(values, ", ")
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to re-insert members: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit reorder: %w", err)
	}
	return nil
}

// CountMembers returns the number of members in a collection.
func (r *collectionRepository) CountMembers(ctx context.Context, collectionID string) (int32, error) {
	var count int32
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collection_templates WHERE collection_id = $1`, collectionID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count collection members: %w", err)
	}
	return count, nil
}

// MaxMemberPosition returns the highest position in a collection (-1 when empty).
func (r *collectionRepository) MaxMemberPosition(ctx context.Context, collectionID string) (int32, error) {
	var max sql.NullInt64
	if err := r.db.QueryRowContext(ctx,
		`SELECT MAX(position) FROM collection_templates WHERE collection_id = $1`, collectionID,
	).Scan(&max); err != nil {
		return 0, fmt.Errorf("failed to get max member position: %w", err)
	}
	if !max.Valid {
		return -1, nil
	}
	return int32(max.Int64), nil
}

// GetMemberPosition returns a member's position, or -1 when not present.
func (r *collectionRepository) GetMemberPosition(ctx context.Context, collectionID, templateID string) (int32, error) {
	var pos int32
	err := r.db.QueryRowContext(ctx,
		`SELECT position FROM collection_templates WHERE collection_id = $1 AND template_id = $2`,
		collectionID, templateID,
	).Scan(&pos)
	if err == sql.ErrNoRows {
		return -1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get member position: %w", err)
	}
	return pos, nil
}

const collectionMemberColumns = `
	t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags, t.category, t.language,
	t.like_count, t.favorite_count, t.comment_count, t.view_count, t.use_count,
	t.moderation_status, t.published_at,
	t.content_warnings, t.locked_warnings,
	t.created_at, t.updated_at,
	ct.position,
	(t.visibility = 'public') AS is_public`

// ListMembers returns a collection's members. ownerView=true returns all
// members (flagged is_public); otherwise only public templates.
func (r *collectionRepository) ListMembers(ctx context.Context, collectionID string, ownerView bool) ([]*models.CollectionMember, error) {
	query := `
		SELECT ` + collectionMemberColumns + `
		FROM collection_templates ct
		JOIN templates t ON t.id = ct.template_id
		WHERE ct.collection_id = $1`
	if !ownerView {
		query += ` AND t.visibility = 'public' AND t.moderation_status = 'normal'
		           AND NOT EXISTS (SELECT 1 FROM users bu WHERE bu.id = t.owner_id AND bu.status = 'banned')`
	}
	query += ` ORDER BY ct.position, ct.added_at`

	rows, err := r.db.QueryContext(ctx, query, collectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list collection members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var members []*models.CollectionMember
	for rows.Next() {
		var m models.CollectionMember
		if err := rows.Scan(
			&m.Template.ID, &m.Template.OwnerID, &m.Template.Title, &m.Template.Description,
			&m.Template.Visibility, &m.Template.Type, &m.Template.Tags, &m.Template.Category,
			&m.Template.Language, &m.Template.LikeCount, &m.Template.FavoriteCount,
			&m.Template.CommentCount, &m.Template.ViewCount, &m.Template.UseCount,
			&m.Template.ModerationStatus, &m.Template.PublishedAt,
			&m.Template.ContentWarnings, &m.Template.LockedWarnings,
			&m.Template.CreatedAt, &m.Template.UpdatedAt,
			&m.Position, &m.IsPublic,
		); err != nil {
			return nil, fmt.Errorf("failed to scan collection member: %w", err)
		}
		members = append(members, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate collection members: %w", err)
	}
	return members, nil
}

// ListContainingTemplate returns the caller's active collections that contain
// a given template (caller-scoped).
func (r *collectionRepository) ListContainingTemplate(ctx context.Context, ownerID, templateID string) ([]*models.Collection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.`+collectionColumns+`,
		       (SELECT COUNT(*) FROM collection_templates ct WHERE ct.collection_id = c.id) AS template_count
		FROM collections c
		JOIN collection_templates ct ON ct.collection_id = c.id
		WHERE c.owner_id = $1 AND c.status = 'active' AND ct.template_id = $2
		ORDER BY c.created_at DESC`, ownerID, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections containing template: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cols []*models.Collection
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Description, &c.Visibility,
			&c.Status, &c.CreatedAt, &c.UpdatedAt, &c.TemplateCount); err != nil {
			return nil, fmt.Errorf("failed to scan containing collection: %w", err)
		}
		cols = append(cols, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate containing collections: %w", err)
	}
	return cols, nil
}
