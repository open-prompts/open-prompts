package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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
	CountTemplates(ctx context.Context, filters map[string]interface{}) (int32, error)
	ListRelated(ctx context.Context, templateID string, tpl *models.Template, currentUserID string, limit int) ([]*models.Template, error)
	ListMoreFromAuthor(ctx context.Context, templateID, ownerID, currentUserID string, limit int) ([]*models.Template, error)
	RateTemplate(ctx context.Context, userID, templateID string, rating int32) (sum int64, count int32, err error)
	DeleteRating(ctx context.Context, userID, templateID string) (sum int64, count int32, err error)
	GetRatingDistribution(ctx context.Context, templateID string) ([5]int32, error)
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
			owner_id, title, description, visibility, type, tags, category, language,
			moderation_status, published_at, content_warnings, locked_warnings, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		) RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		t.OwnerID, t.Title, t.Description, t.Visibility, t.Type, pq.Array(t.Tags), t.Category, t.Language,
		t.ModerationStatus, t.PublishedAt, pq.Array(warnArray(t.ContentWarnings)), pq.Array(warnArray(t.LockedWarnings)),
		t.CreatedAt, t.UpdatedAt,
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
		SET title = $1, description = $2, visibility = $3, tags = $4, category = $5, language = $6,
		    moderation_status = $7, published_at = $8, content_warnings = $9, locked_warnings = $10,
		    updated_at = $11
		WHERE id = $12
	`
	_, err := r.db.ExecContext(ctx, query,
		t.Title, t.Description, t.Visibility, pq.Array(t.Tags), t.Category, t.Language,
		t.ModerationStatus, t.PublishedAt, pq.Array(warnArray(t.ContentWarnings)), pq.Array(warnArray(t.LockedWarnings)),
		t.UpdatedAt, t.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}
	return nil
}

// warnArray returns a non-nil slice for the warning columns so an empty set
// is stored as '{}' (NOT NULL) rather than NULL.
func warnArray(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
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
			t.like_count, t.favorite_count, t.comment_count, t.view_count, t.use_count,
			t.rating_sum, t.rating_count,
			t.moderation_status, t.published_at,
			t.content_warnings, t.locked_warnings,
			t.created_at, t.updated_at,
			CASE WHEN tl.user_id IS NOT NULL THEN true ELSE false END as is_liked,
			CASE WHEN tf.user_id IS NOT NULL THEN true ELSE false END as is_favorited,
			COALESCE(mr.rating, 0) AS my_rating
		FROM templates t
		LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $2
		LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $2
		LEFT JOIN template_ratings mr ON t.id = mr.template_id AND mr.user_id = $2
		WHERE t.id = $1
	`
	var t models.Template
	err := r.db.QueryRowContext(ctx, query, id, currentUserID).Scan(
		&t.ID, &t.OwnerID, &t.Title, &t.Description, &t.Visibility, &t.Type,
		&t.Tags, &t.Category, &t.Language, &t.LikeCount, &t.FavoriteCount, &t.CommentCount,
		&t.ViewCount, &t.UseCount,
		&t.RatingSum, &t.RatingCount,
		&t.ModerationStatus, &t.PublishedAt, &t.ContentWarnings, &t.LockedWarnings,
		&t.CreatedAt, &t.UpdatedAt,
		&t.IsLiked, &t.IsFavorited,
		&t.MyRating,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("template not found")
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	// Public templates of banned owners are hidden from everyone except the
	// owner themselves (who cannot authenticate while banned anyway).
	if t.Visibility == "public" && t.OwnerID != currentUserID {
		var banned bool
		err := r.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 AND status = 'banned')`, t.OwnerID,
		).Scan(&banned)
		if err == nil && banned {
			return nil, fmt.Errorf("template not found")
		}
	}
	// Gated templates (in_review/rejected/unpublished_by_admin/soft_deleted)
	// are visible only to their owner (admins use the admin repository,
	// which does not filter). 404-equivalent for everyone else — no status
	// leak. The locked state is intentionally NOT hidden — locked templates
	// stay publicly visible (FR-3).
	if t.ModerationStatus != "normal" && t.ModerationStatus != "locked" && t.OwnerID != currentUserID {
		return nil, fmt.Errorf("template not found")
	}
	return &t, nil
}

// trendingWindow is the trailing window for the "trending" sort
// (likes + favorites). Deliberately a server-side constant, not tunable.
// Expressed as a Postgres interval literal because a bare numeric bind
// parameter is interpreted in seconds and 7d of nanoseconds overflows it.
const trendingWindow = "7 days"

// defaultWarningTaxonomy is the built-in fallback warning taxonomy used by
// the exclude_warned filter when the platform_settings value is unavailable.
// The service normally passes the live taxonomy; this is a resilience
// fallback so the filter never inlines user input.
var defaultWarningTaxonomy = []string{"mature", "violence", "profanity", "sensitive"}

// escapeLike backslash-escapes LIKE metacharacters (\, %, _) so user input
// is always matched literally inside an ILIKE pattern.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// templateOrderBy maps a whitelisted sort value to a hardcoded ORDER BY
// clause. User input never reaches SQL text — anything unrecognized falls
// back to the default newest ordering. The top_rated threshold is a bound
// parameter (never inlined); it is appended to args and the next placeholder
// id is returned. For non-top_rated sorts the threshold is ignored and argID
// is returned unchanged.
func templateOrderBy(sort string, threshold int, args []interface{}, argID int) (string, []interface{}, int) {
	switch sort {
	case "most_liked":
		return " ORDER BY t.like_count DESC, t.created_at DESC", args, argID
	case "most_favorited":
		return " ORDER BY t.favorite_count DESC, t.created_at DESC", args, argID
	case "trending":
		return " ORDER BY (COALESCE(el.cnt, 0) + COALESCE(ef.cnt, 0)) DESC, t.created_at DESC", args, argID
	case "top_rated":
		order := fmt.Sprintf(` ORDER BY
			(t.rating_count >= $%d) DESC,
			CASE WHEN t.rating_count >= $%d
			     THEN t.rating_sum::float / GREATEST(t.rating_count, 1) END DESC NULLS LAST,
			t.rating_count DESC,
			t.created_at DESC`, argID, argID)
		args = append(args, threshold)
		return order, args, argID + 1
	case "updated":
		return " ORDER BY t.updated_at DESC", args, argID
	default: // "newest" and any invalid value
		return " ORDER BY t.created_at DESC", args, argID
	}
}

// trendingJoins returns the derived-table joins backing the trending sort.
// They are only appended when the sort is trending so every other query
// stays byte-identical to the pre-feature form.
func trendingJoins() string {
	return `
		LEFT JOIN (SELECT template_id, COUNT(*) AS cnt FROM template_likes
		           WHERE created_at > now() - $%d::interval
		           GROUP BY template_id) el ON el.template_id = t.id
		LEFT JOIN (SELECT template_id, COUNT(*) AS cnt FROM template_favorites
		           WHERE created_at > now() - $%d::interval
		           GROUP BY template_id) ef ON ef.template_id = t.id
	`
}

// buildTemplateConditions assembles the shared WHERE conditions (and the
// trending FROM joins) from the filters map. List and CountTemplates both
// call it so counts can never diverge from results. It returns the SQL
// fragment, the extended args slice, and the next placeholder id.
func buildTemplateConditions(filters map[string]interface{}, args []interface{}, argID int) (string, []interface{}, int) {
	query := ""

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
	// Hide public templates of banned owners from the public square. A
	// private listing (owner's own view) is unaffected.
	if val, ok := filters["visibility"]; !ok || val == "" || val == "public" {
		query += " AND NOT EXISTS (SELECT 1 FROM users bu WHERE bu.id = t.owner_id AND bu.status = 'banned')"
		// Moderation filter mirrors the banned-owner pattern: gated
		// (in_review/rejected/unpublished_by_admin/soft_deleted) templates
		// are hidden from every public stream except their owner's. $1 is
		// current_user_id ("" for anonymous), so the bypass never matches
		// for anonymous callers. The locked state is intentionally NOT
		// hidden — locked templates stay publicly visible (FR-3).
		query += " AND (t.moderation_status IN ('normal', 'locked') OR t.owner_id = $1)"
		// exclude_warned applies to the public stream only (FR-6): when
		// true, drop templates carrying any taxonomy warning. The taxonomy
		// array is a bound parameter, never inlined (constraint 7).
		if val, ok := filters["exclude_warned"]; ok && val.(bool) {
			query += fmt.Sprintf(" AND NOT (t.content_warnings && $%d)", argID)
			args = append(args, pq.Array(defaultWarningTaxonomy))
			argID++
		}
	}
	// Keyword: case-insensitive phrase match across title, description,
	// category, individual tag values, and latest-version content. The
	// pattern is a bound parameter with LIKE metacharacters escaped.
	if kw, ok := filters["keyword"].(string); ok && kw != "" {
		pattern := "%" + escapeLike(kw) + "%"
		// All five ILIKE references share a single placeholder, so the
		// pattern is bound once.
		query += fmt.Sprintf(`
			AND (
				t.title ILIKE $%d
				OR t.description ILIKE $%d
				OR t.category ILIKE $%d
				OR EXISTS (SELECT 1 FROM unnest(t.tags) AS tg WHERE tg ILIKE $%d)
				OR EXISTS (SELECT 1 FROM template_versions tv
				           WHERE tv.id = (SELECT tv2.id FROM template_versions tv2
				                          WHERE tv2.template_id = t.id
				                          ORDER BY tv2.version DESC LIMIT 1)
				             AND tv.content ILIKE $%d)
			)`, argID, argID, argID, argID, argID)
		args = append(args, pattern)
		argID++
	}
	return query, args, argID
}

// templateFromClause returns the FROM clause (with viewer LEFT JOINs and,
// for the trending sort, the windowed engagement joins). The trending
// interval is a bound parameter appended to args; the next placeholder id
// is returned.
func templateFromClause(sort string, args []interface{}, argID int) (string, []interface{}, int) {
	from := `
		FROM templates t
		LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $1
		LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $1
		LEFT JOIN template_ratings mr ON t.id = mr.template_id AND mr.user_id = $1
	`
	if sort == "trending" {
		from += fmt.Sprintf(trendingJoins(), argID, argID+1)
		args = append(args, trendingWindow, trendingWindow)
		argID += 2
	}
	return from, args, argID
}

// List retrieves a list of templates based on filters and pagination.
func (r *templateRepository) List(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*models.Template, error) {
	currentUserID := ""
	if val, ok := filters["current_user_id"]; ok {
		currentUserID = val.(string)
	}
	sort, _ := filters["sort"].(string)

	query := `
		SELECT
			t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags, t.category, t.language,
			t.like_count, t.favorite_count, t.comment_count, t.view_count, t.use_count,
			t.rating_sum, t.rating_count,
			t.moderation_status, t.published_at,
			t.content_warnings, t.locked_warnings,
			t.created_at, t.updated_at,
			CASE WHEN tl.user_id IS NOT NULL THEN true ELSE false END as is_liked,
			CASE WHEN tf.user_id IS NOT NULL THEN true ELSE false END as is_favorited,
			COALESCE(mr.rating, 0) AS my_rating,
			ou.display_name AS owner_display_name, COALESCE(ou.avatar, '') AS owner_avatar
	`
	args := []interface{}{currentUserID}
	argID := 2

	from, args, argID := templateFromClause(sort, args, argID)
	query += from + " JOIN users ou ON ou.id = t.owner_id WHERE 1=1"

	conditions, args, argID := buildTemplateConditions(filters, args, argID)
	query += conditions

	// top_rated threshold (server config, default 5) is a bound parameter.
	threshold := 5
	if val, ok := filters["top_rated_threshold"]; ok {
		if n, ok := val.(int); ok && n > 0 {
			threshold = n
		}
	}
	order, args, argID := templateOrderBy(sort, threshold, args, argID)
	query += order
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argID, argID+1)
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
			&t.Tags, &t.Category, &t.Language, &t.LikeCount, &t.FavoriteCount, &t.CommentCount,
			&t.ViewCount, &t.UseCount,
			&t.RatingSum, &t.RatingCount,
			&t.ModerationStatus, &t.PublishedAt, &t.ContentWarnings, &t.LockedWarnings,
			&t.CreatedAt, &t.UpdatedAt,
			&t.IsLiked, &t.IsFavorited,
			&t.MyRating,
			&t.OwnerDisplayName, &t.OwnerAvatar,
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

// CountTemplates returns the total number of templates matching the given
// filters, using the exact same condition builder as List so counts and
// results can never diverge.
func (r *templateRepository) CountTemplates(ctx context.Context, filters map[string]interface{}) (int32, error) {
	currentUserID := ""
	if val, ok := filters["current_user_id"]; ok {
		currentUserID = val.(string)
	}
	sort, _ := filters["sort"].(string)

	query := `SELECT COUNT(*)`
	args := []interface{}{currentUserID}
	argID := 2

	from, args, argID := templateFromClause(sort, args, argID)
	query += from + " WHERE 1=1"

	conditions, args, _ := buildTemplateConditions(filters, args, argID)
	query += conditions

	var count int32
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count templates: %w", err)
	}
	return count, nil
}

// ListCategories retrieves all categories and their template counts.
func (r *templateRepository) ListCategories(ctx context.Context, filters map[string]interface{}) ([]*models.CategoryStat, error) {
	query := `
		SELECT category, COUNT(*) as count
		FROM templates
		WHERE category IS NOT NULL AND category != ''
		AND NOT EXISTS (SELECT 1 FROM users bu WHERE bu.id = templates.owner_id AND bu.status = 'banned')
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
			WHERE 1=1 AND NOT EXISTS (SELECT 1 FROM users bu WHERE bu.id = templates.owner_id AND bu.status = 'banned') %s
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

// RateTemplate upserts the caller's 1-5 rating for a template and maintains
// the denormalized aggregates transactionally. Returns the fresh
// (rating_sum, rating_count). The delta math is done in a single CTE so the
// read-modify-write is atomic: the previous rating is captured FOR UPDATE,
// the upsert reports whether a row was inserted, and the templates row is
// updated with the exact delta (floored at 0).
func (r *templateRepository) RateTemplate(ctx context.Context, userID, templateID string, rating int32) (int64, int32, error) {
	query := `
		WITH prev AS (
			SELECT rating FROM template_ratings
			WHERE user_id = $1 AND template_id = $2 FOR UPDATE
		), upsert AS (
			INSERT INTO template_ratings (user_id, template_id, rating)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, template_id)
			DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
			RETURNING rating, (xmax = 0) AS inserted
		)
		UPDATE templates t
		SET rating_sum = GREATEST(t.rating_sum + $3 - COALESCE((SELECT rating FROM prev), 0), 0),
		    rating_count = t.rating_count + CASE WHEN (SELECT inserted FROM upsert) THEN 1 ELSE 0 END
		WHERE t.id = $2
		RETURNING t.rating_sum, t.rating_count
	`
	var sum int64
	var count int32
	err := r.db.QueryRowContext(ctx, query, userID, templateID, rating).Scan(&sum, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to rate template: %w", err)
	}
	return sum, count, nil
}

// DeleteRating removes the caller's rating for a template (idempotent) and
// maintains the denormalized aggregates transactionally. A missing row is a
// no-op returning the current aggregates.
func (r *templateRepository) DeleteRating(ctx context.Context, userID, templateID string) (int64, int32, error) {
	query := `
		WITH removed AS (
			DELETE FROM template_ratings
			WHERE user_id = $1 AND template_id = $2
			RETURNING rating
		)
		UPDATE templates t
		SET rating_sum = GREATEST(t.rating_sum - COALESCE((SELECT rating FROM removed), 0), 0),
		    rating_count = t.rating_count - CASE WHEN EXISTS (SELECT 1 FROM removed) THEN 1 ELSE 0 END
		WHERE t.id = $2
		RETURNING t.rating_sum, t.rating_count
	`
	var sum int64
	var count int32
	err := r.db.QueryRowContext(ctx, query, userID, templateID).Scan(&sum, &count)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to delete rating: %w", err)
	}
	return sum, count, nil
}

// GetRatingDistribution returns the per-star counts for a template as a
// fixed 5-element array (index 0 -> 1★ ... index 4 -> 5★).
func (r *templateRepository) GetRatingDistribution(ctx context.Context, templateID string) ([5]int32, error) {
	var dist [5]int32
	rows, err := r.db.QueryContext(ctx,
		`SELECT rating, COUNT(*) FROM template_ratings WHERE template_id = $1 GROUP BY rating`,
		templateID,
	)
	if err != nil {
		return dist, fmt.Errorf("failed to get rating distribution: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rating int32
		var count int32
		if err := rows.Scan(&rating, &count); err != nil {
			return dist, fmt.Errorf("failed to scan rating distribution: %w", err)
		}
		if rating >= 1 && rating <= 5 {
			dist[rating-1] = count
		}
	}
	if err := rows.Err(); err != nil {
		return dist, fmt.Errorf("rows error: %w", err)
	}
	return dist, nil
}

// relatedSelectColumns is the shared SELECT column list (with viewer
// like/favorite LEFT JOINs) used by the discovery queries. It mirrors the
// List query's projection so cards render the same shape.
const relatedSelectColumns = `
		t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags, t.category, t.language,
		t.like_count, t.favorite_count, t.comment_count, t.view_count, t.use_count,
		t.rating_sum, t.rating_count,
		t.moderation_status, t.published_at,
		t.content_warnings, t.locked_warnings,
		t.created_at, t.updated_at,
		CASE WHEN tl.user_id IS NOT NULL THEN true ELSE false END as is_liked,
		CASE WHEN tf.user_id IS NOT NULL THEN true ELSE false END as is_favorited,
		COALESCE(mr.rating, 0) AS my_rating,
		ou.display_name AS owner_display_name, COALESCE(ou.avatar, '') AS owner_avatar
`

// scanTemplateRow scans one row into a *models.Template using the same
// projection as List. It is shared by ListRelated and ListMoreFromAuthor.
func scanTemplateRow(scan func(dest ...interface{}) error) (*models.Template, error) {
	var t models.Template
	if err := scan(
		&t.ID, &t.OwnerID, &t.Title, &t.Description, &t.Visibility, &t.Type,
		&t.Tags, &t.Category, &t.Language, &t.LikeCount, &t.FavoriteCount, &t.CommentCount,
		&t.ViewCount, &t.UseCount,
		&t.RatingSum, &t.RatingCount,
		&t.ModerationStatus, &t.PublishedAt, &t.ContentWarnings, &t.LockedWarnings,
		&t.CreatedAt, &t.UpdatedAt,
		&t.IsLiked, &t.IsFavorited,
		&t.MyRating,
		&t.OwnerDisplayName, &t.OwnerAvatar,
	); err != nil {
		return nil, err
	}
	return &t, nil
}

// relatedQuery builds the SQL and bound args for ListRelated. Extracted from
// the DB call so the parameter order/count and the empty-tags edge can be
// unit-tested without a live database.
func relatedQuery(templateID string, tpl *models.Template, currentUserID string, limit int) (string, []interface{}) {
	query := `
		SELECT` + relatedSelectColumns + `
		FROM templates t
		LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $2
		LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $2
		LEFT JOIN template_ratings mr ON t.id = mr.template_id AND mr.user_id = $2
		JOIN users ou ON ou.id = t.owner_id
		WHERE t.visibility = 'public'
		  AND t.id != $1
		  AND NOT EXISTS (SELECT 1 FROM users bu
		                  WHERE bu.id = t.owner_id AND bu.status = 'banned')
		  AND (t.moderation_status IN ('normal', 'locked') OR t.owner_id = $2)
		ORDER BY
		    (CASE WHEN t.category = $3 THEN 3 ELSE 0 END
		    + CASE WHEN t.tags && $4 THEN 2 * cardinality(ARRAY(
		              SELECT unnest(t.tags) INTERSECT SELECT unnest($4))) ELSE 0 END
		    + CASE WHEN t.language = $5 THEN 1 ELSE 0 END
		    + CASE WHEN t.owner_id = $6 THEN 0.5 ELSE 0 END) DESC,
		    t.like_count DESC,
		    t.updated_at DESC,
		    t.id ASC
		LIMIT $7
	`
	args := []interface{}{
		templateID,
		currentUserID,
		tpl.Category.String,
		pq.Array(tpl.Tags),
		tpl.Language,
		tpl.OwnerID,
		limit,
	}
	return query, args
}

// ListRelated returns up to `limit` public templates related to the given
// template, scored by shared category (3), shared tags (2 per shared tag),
// same language (1), and same author (0.5), then like_count DESC, updated_at
// DESC, id ASC. The current template, private templates, and banned-owner
// templates are excluded. currentUserID (may be "") drives the viewer
// like/favorite joins.
func (r *templateRepository) ListRelated(ctx context.Context, templateID string, tpl *models.Template, currentUserID string, limit int) ([]*models.Template, error) {
	query, args := relatedQuery(templateID, tpl, currentUserID, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query related templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var templates []*models.Template
	for rows.Next() {
		t, err := scanTemplateRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan related template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return templates, nil
}

// authorQuery builds the SQL and bound args for ListMoreFromAuthor. Extracted
// for unit testing of parameter order/count.
func authorQuery(templateID, ownerID, currentUserID string, limit int) (string, []interface{}) {
	query := `
		SELECT` + relatedSelectColumns + `
		FROM templates t
		LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $2
		LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $2
		LEFT JOIN template_ratings mr ON t.id = mr.template_id AND mr.user_id = $2
		JOIN users ou ON ou.id = t.owner_id
		WHERE t.owner_id = $3
		  AND t.visibility = 'public'
		  AND t.id != $1
		  AND (t.moderation_status IN ('normal', 'locked') OR t.owner_id = $2)
		ORDER BY t.like_count DESC, t.updated_at DESC, t.id ASC
		LIMIT $4
	`
	args := []interface{}{templateID, currentUserID, ownerID, limit}
	return query, args
}

// ListMoreFromAuthor returns up to `limit` of the author's other public
// templates, most-liked first. The current template is excluded. The
// banned-owner exclusion is unnecessary here because the caller reached this
// code only through Get, which already 404s banned-owner templates.
func (r *templateRepository) ListMoreFromAuthor(ctx context.Context, templateID, ownerID, currentUserID string, limit int) ([]*models.Template, error) {
	query, args := authorQuery(templateID, ownerID, currentUserID, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query more from author: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var templates []*models.Template
	for rows.Next() {
		t, err := scanTemplateRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan author template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return templates, nil
}
