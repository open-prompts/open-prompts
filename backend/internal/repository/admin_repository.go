package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"open-prompts/backend/internal/models"

	"github.com/lib/pq"
)

var (
	ErrAuditLogNotFound    = errors.New("audit log not found")
	ErrTemplateNotFound    = errors.New("template not found")
	ErrTemplateNotInReview = errors.New("template not in review")
)

// UserFilters describes the admin user list query parameters.
type UserFilters struct {
	Search string // ILIKE %s% over id, email, display_name
	Status string // "" | active | banned
	Role   string // "" | user | admin
	Sort   string // newest | oldest | recently_updated
	Limit  int
	Offset int
}

// TemplateFilters describes the admin template list query parameters.
type TemplateFilters struct {
	Search     string // ILIKE %s% over title, owner id, owner display_name
	Visibility string // "" | public | private
	OwnerID    string // deep-link filter from user detail
	Sort       string // recently_updated | newest | oldest | most_likes
	Warning    string // "" = any | "none" | a taxonomy key
	Limit      int
	Offset     int
}

// AuditFilters describes the admin audit log query parameters.
type AuditFilters struct {
	Action string // "" | ban | unban | role_change
	Actor  string // matches actor_id or actor_email
	Limit  int
	Offset int
}

// CommentFilters describes the admin comment moderation query parameters.
type CommentFilters struct {
	Search     string // ILIKE %s% over content (wildcards escaped)
	TemplateID string // exact match
	UserID     string // author exact match
	Limit      int
	Offset     int
}

// AdminRepository is the data-access interface for the admin console.
type AdminRepository interface {
	// Users
	ListUsers(ctx context.Context, f UserFilters) ([]*models.User, int64, error)
	GetUserDetail(ctx context.Context, userID string) (*models.User, error)
	CountUserTemplates(ctx context.Context, userID string) (int64, error)
	GetUserStats(ctx context.Context, userID string) (*models.UserStats, error)
	ListUserIdentities(ctx context.Context, userID string) ([]*models.OAuthIdentity, error)
	// Templates (admin view: includes private, includes banned owners)
	ListTemplates(ctx context.Context, f TemplateFilters) ([]*models.AdminTemplate, int64, error)
	GetTemplateWithVersions(ctx context.Context, templateID string) (*models.AdminTemplate, []*models.AdminTemplateVersion, error)
	// SetTemplateModeration transitions a template's moderation status.
	// publish=true sets moderation_status='normal' + published_at=now();
	// publish=false sets moderation_status='rejected'. Both are guarded by
	// WHERE moderation_status='in_review' so concurrent decisions are safe.
	SetTemplateModeration(ctx context.Context, templateID string, status string, publish bool) error
	// UpdateTemplateWarnings sets the content_warnings/locked_warnings arrays
	// on a template. Returns ErrTemplateNotFound when the id is unknown.
	UpdateTemplateWarnings(ctx context.Context, templateID string, contentWarnings, lockedWarnings []string) error
	// ListWarningHistory returns the warning-change audit entries for a
	// template (action template.warning_add / template.warning_remove),
	// newest first, paginated.
	ListWarningHistory(ctx context.Context, templateID string, limit, offset int) ([]*models.WarningHistoryEntry, int64, error)
	// Comments (moderation)
	ListComments(ctx context.Context, f CommentFilters) ([]*models.AdminComment, int64, error)
	DeleteComment(ctx context.Context, id string) (bool, int64, error)
	// Stats
	GetStats(ctx context.Context) (*models.AdminStats, error)
	ListRecentSignups(ctx context.Context, limit int) ([]*models.User, error)
	ListRecentPublicTemplates(ctx context.Context, limit int) ([]*models.AdminTemplate, error)
	// Audit
	InsertAuditLog(ctx context.Context, entry *models.AdminAuditLog) error
	ListAuditLogs(ctx context.Context, f AuditFilters) ([]*models.AdminAuditLog, int64, error)
	ListAuditByTarget(ctx context.Context, targetID string, limit int) ([]*models.AdminAuditLog, error)
}

type adminRepository struct {
	db *sql.DB
}

// NewAdminRepository creates a new AdminRepository.
func NewAdminRepository(db *sql.DB) AdminRepository {
	return &adminRepository{db: db}
}

const adminUserColumns = `id, email, mobile, password_hash, display_name, COALESCE(avatar, ''), role, status, approved_template_count, review_gated_remaining, show_warned_content, created_at, updated_at`

func scanAdminUser(row interface{ Scan(...interface{}) error }) (*models.User, error) {
	var u models.User
	if err := row.Scan(
		&u.ID, &u.Email, &u.Mobile, &u.PasswordHash, &u.DisplayName, &u.Avatar,
		&u.Role, &u.Status, &u.ApprovedTemplateCount, &u.ReviewGatedRemaining, &u.ShowWarnedContent,
		&u.CreatedAt, &u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsers returns a page of users plus the total count for the filters.
func (r *adminRepository) ListUsers(ctx context.Context, f UserFilters) ([]*models.User, int64, error) {
	where := ` WHERE 1=1`
	args := []interface{}{}
	argID := 1

	if f.Search != "" {
		where += fmt.Sprintf(` AND (id ILIKE $%d OR email ILIKE $%d OR display_name ILIKE $%d)`, argID, argID, argID)
		args = append(args, "%"+f.Search+"%")
		argID++
	}
	if f.Status != "" {
		where += fmt.Sprintf(` AND status = $%d`, argID)
		args = append(args, f.Status)
		argID++
	}
	if f.Role != "" {
		where += fmt.Sprintf(` AND role = $%d`, argID)
		args = append(args, f.Role)
		argID++
	}

	order := ` ORDER BY created_at DESC`
	switch f.Sort {
	case "oldest":
		order = ` ORDER BY created_at ASC`
	case "recently_updated":
		order = ` ORDER BY updated_at DESC`
	}

	countQuery := `SELECT COUNT(*) FROM users` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	query := `SELECT ` + adminUserColumns + ` FROM users` + where + order +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argID, argID+1)
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var users []*models.User
	for rows.Next() {
		u, err := scanAdminUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return users, total, nil
}

// GetUserDetail fetches a single user by ID.
func (r *adminRepository) GetUserDetail(ctx context.Context, userID string) (*models.User, error) {
	query := `SELECT ` + adminUserColumns + ` FROM users WHERE id = $1`
	u, err := scanAdminUser(r.db.QueryRowContext(ctx, query, userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return u, nil
}

// CountUserTemplates returns the number of templates owned by a user.
func (r *adminRepository) CountUserTemplates(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM templates WHERE owner_id = $1`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count user templates: %w", err)
	}
	return count, nil
}

// GetUserStats aggregates per-user statistics.
func (r *adminRepository) GetUserStats(ctx context.Context, userID string) (*models.UserStats, error) {
	query := `
SELECT
	(SELECT COUNT(*) FROM templates WHERE owner_id = $1) AS templates,
	COALESCE((SELECT SUM(like_count) FROM templates WHERE owner_id = $1), 0) AS likes_received,
	COALESCE((SELECT SUM(favorite_count) FROM templates WHERE owner_id = $1), 0) AS favorites_received,
	(SELECT COUNT(*) FROM prompts WHERE owner_id = $1) AS prompts_created
`
	var s models.UserStats
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&s.Templates, &s.LikesReceived, &s.FavoritesReceived, &s.PromptsCreated)
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}
	return &s, nil
}

// ListUserIdentities returns the OAuth identities linked to a user.
func (r *adminRepository) ListUserIdentities(ctx context.Context, userID string) ([]*models.OAuthIdentity, error) {
	query := `SELECT provider, created_at FROM user_identities WHERE user_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user identities: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var identities []*models.OAuthIdentity
	for rows.Next() {
		i := &models.OAuthIdentity{}
		if err := rows.Scan(&i.Provider, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan identity: %w", err)
		}
		identities = append(identities, i)
	}
	return identities, nil
}

const adminTemplateColumns = `t.id, t.owner_id, COALESCE(u.display_name, ''), t.title, t.description, t.visibility, t.type, t.language, t.tags, t.like_count, t.favorite_count, t.content_warnings, t.locked_warnings, t.created_at, t.updated_at`

func scanAdminTemplate(row interface{ Scan(...interface{}) error }) (*models.AdminTemplate, error) {
	var t models.AdminTemplate
	if err := row.Scan(
		&t.ID, &t.OwnerID, &t.OwnerDisplayName, &t.Title, &t.Description,
		&t.Visibility, &t.Type, &t.Language, &t.Tags, &t.LikeCount, &t.FavoriteCount,
		&t.ContentWarnings, &t.LockedWarnings,
		&t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTemplates returns a page of ALL templates (public and private) plus the
// total count for the filters.
func (r *adminRepository) ListTemplates(ctx context.Context, f TemplateFilters) ([]*models.AdminTemplate, int64, error) {
	where := ` WHERE 1=1`
	args := []interface{}{}
	argID := 1

	if f.Search != "" {
		where += fmt.Sprintf(` AND (t.title ILIKE $%d OR t.owner_id ILIKE $%d OR u.display_name ILIKE $%d)`, argID, argID, argID)
		args = append(args, "%"+f.Search+"%")
		argID++
	}
	if f.Visibility != "" {
		where += fmt.Sprintf(` AND t.visibility = $%d`, argID)
		args = append(args, f.Visibility)
		argID++
	}
	if f.OwnerID != "" {
		where += fmt.Sprintf(` AND t.owner_id = $%d`, argID)
		args = append(args, f.OwnerID)
		argID++
	}
	// Warning-state filter (FR-9): "" = any, "none" = no warnings, a
	// taxonomy key = contains that warning. Values are bound parameters.
	switch f.Warning {
	case "none":
		where += ` AND (t.content_warnings = '{}' OR t.content_warnings IS NULL)`
	case "":
		// any — no clause
	default:
		where += fmt.Sprintf(` AND t.content_warnings @> ARRAY[$%d]`, argID)
		args = append(args, f.Warning)
		argID++
	}

	order := ` ORDER BY t.updated_at DESC`
	switch f.Sort {
	case "newest":
		order = ` ORDER BY t.created_at DESC`
	case "oldest":
		order = ` ORDER BY t.created_at ASC`
	case "most_likes":
		order = ` ORDER BY t.like_count DESC, t.updated_at DESC`
	}

	countQuery := `SELECT COUNT(*) FROM templates t LEFT JOIN users u ON u.id = t.owner_id` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count templates: %w", err)
	}

	query := `SELECT ` + adminTemplateColumns + `
FROM templates t
LEFT JOIN users u ON u.id = t.owner_id` + where + order +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argID, argID+1)
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list templates: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var templates []*models.AdminTemplate
	for rows.Next() {
		t, err := scanAdminTemplate(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return templates, total, nil
}

// GetTemplateWithVersions fetches any template (including private) with all
// its versions and the alias (if any) pointing at each version.
func (r *adminRepository) GetTemplateWithVersions(ctx context.Context, templateID string) (*models.AdminTemplate, []*models.AdminTemplateVersion, error) {
	query := `SELECT ` + adminTemplateColumns + `
FROM templates t
LEFT JOIN users u ON u.id = t.owner_id
WHERE t.id = $1`

	t, err := scanAdminTemplate(r.db.QueryRowContext(ctx, query, templateID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrTemplateNotFound
		}
		return nil, nil, fmt.Errorf("failed to get template: %w", err)
	}

	versionQuery := `
SELECT tv.id, tv.version, tv.content, tv.created_at,
	COALESCE((SELECT a.alias_name FROM template_aliases a WHERE a.version_id = tv.id ORDER BY a.created_at ASC LIMIT 1), '') AS alias_name
FROM template_versions tv
WHERE tv.template_id = $1
ORDER BY tv.version DESC
`
	rows, err := r.db.QueryContext(ctx, versionQuery, templateID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list versions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var versions []*models.AdminTemplateVersion
	for rows.Next() {
		v := &models.AdminTemplateVersion{}
		if err := rows.Scan(&v.ID, &v.Version, &v.Content, &v.CreatedAt, &v.AliasName); err != nil {
			return nil, nil, fmt.Errorf("failed to scan version: %w", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}

	return t, versions, nil
}

// GetStats computes the dashboard KPIs in a single query.
func (r *adminRepository) GetStats(ctx context.Context) (*models.AdminStats, error) {
	query := `
SELECT
	(SELECT COUNT(*) FROM users) AS total_users,
	(SELECT COUNT(*) FROM users WHERE status = 'active') AS active_users,
	(SELECT COUNT(*) FROM users WHERE status = 'banned') AS banned_users,
	(SELECT COUNT(*) FROM templates) AS total_templates,
	(SELECT COUNT(*) FROM templates t WHERE t.visibility = 'public' AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = t.owner_id AND u.status = 'banned')) AS public_templates,
	(SELECT COUNT(*) FROM prompts) AS prompts_created
`
	var s models.AdminStats
	err := r.db.QueryRowContext(ctx, query).Scan(
		&s.TotalUsers, &s.ActiveUsers, &s.BannedUsers,
		&s.TotalTemplates, &s.PublicTemplates, &s.PromptsCreated,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}
	return &s, nil
}

// ListRecentSignups returns the most recently created users.
func (r *adminRepository) ListRecentSignups(ctx context.Context, limit int) ([]*models.User, error) {
	query := `SELECT ` + adminUserColumns + ` FROM users ORDER BY created_at DESC LIMIT $1`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent signups: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var users []*models.User
	for rows.Next() {
		u, err := scanAdminUser(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

// ListRecentPublicTemplates returns the most recently updated public templates
// of non-banned owners.
func (r *adminRepository) ListRecentPublicTemplates(ctx context.Context, limit int) ([]*models.AdminTemplate, error) {
	query := `SELECT ` + adminTemplateColumns + `
FROM templates t
LEFT JOIN users u ON u.id = t.owner_id
WHERE t.visibility = 'public'
  AND NOT EXISTS (SELECT 1 FROM users bu WHERE bu.id = t.owner_id AND bu.status = 'banned')
ORDER BY t.updated_at DESC
LIMIT $1`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent templates: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var templates []*models.AdminTemplate
	for rows.Next() {
		t, err := scanAdminTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		templates = append(templates, t)
	}
	return templates, nil
}

// SetTemplateModeration transitions a template out of in_review. The
// WHERE moderation_status='in_review' guard makes concurrent decisions
// idempotent at the template level.
func (r *adminRepository) SetTemplateModeration(ctx context.Context, templateID string, status string, publish bool) error {
	var query string
	var args []interface{}
	if publish {
		query = `
			UPDATE templates
			SET moderation_status = $1, published_at = now(), updated_at = now()
			WHERE id = $2 AND moderation_status = 'in_review'
		`
		args = []interface{}{status, templateID}
	} else {
		query = `
			UPDATE templates
			SET moderation_status = $1, updated_at = now()
			WHERE id = $2 AND moderation_status = 'in_review'
		`
		args = []interface{}{status, templateID}
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update template moderation: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update template moderation: %w", err)
	}
	if rows == 0 {
		return ErrTemplateNotInReview
	}
	return nil
}

// UpdateTemplateWarnings sets the warning arrays on a template. The
// WHERE id = $1 guard returns ErrTemplateNotFound when the id is unknown.
func (r *adminRepository) UpdateTemplateWarnings(ctx context.Context, templateID string, contentWarnings, lockedWarnings []string) error {
	query := `
		UPDATE templates
		SET content_warnings = $1, locked_warnings = $2, updated_at = now()
		WHERE id = $3
	`
	res, err := r.db.ExecContext(ctx, query, pq.Array(warnArray(contentWarnings)), pq.Array(warnArray(lockedWarnings)), templateID)
	if err != nil {
		return fmt.Errorf("failed to update template warnings: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update template warnings: %w", err)
	}
	if rows == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

// ListWarningHistory returns the warning-change audit entries for a template,
// newest first, paginated. The warnings column is parsed from the reason
// field, which the service stores as a JSON array of the changed warnings.
func (r *adminRepository) ListWarningHistory(ctx context.Context, templateID string, limit, offset int) ([]*models.WarningHistoryEntry, int64, error) {
	where := ` WHERE target_type = 'template' AND target_id = $1
	           AND action IN ('template.warning_add', 'template.warning_remove')`
	args := []interface{}{templateID}
	argID := 2

	countQuery := `SELECT COUNT(*) FROM admin_audit_logs` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count warning history: %w", err)
	}

	query := `SELECT id, actor_id, actor_email, action, reason, created_at
FROM admin_audit_logs` + where + ` ORDER BY created_at DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argID, argID+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list warning history: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*models.WarningHistoryEntry
	for rows.Next() {
		e := &models.WarningHistoryEntry{}
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorEmail, &e.Action, &e.Reason, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan warning history: %w", err)
		}
		// The reason field carries a JSON array of the changed warnings
		// (service-side), so parse it back into the Warnings slice.
		var warnings []string
		if err := json.Unmarshal([]byte(e.Reason), &warnings); err == nil {
			e.Warnings = warnings
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return entries, total, nil
}

// InsertAuditLog appends an entry to the audit trail.
func (r *adminRepository) InsertAuditLog(ctx context.Context, entry *models.AdminAuditLog) error {
	query := `
INSERT INTO admin_audit_logs (actor_id, actor_email, action, target_type, target_id, reason)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at`

	err := r.db.QueryRowContext(ctx, query,
		entry.ActorID, entry.ActorEmail, entry.Action, entry.TargetType, entry.TargetID, entry.Reason,
	).Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}
	zap.S().Infof("AdminRepository.InsertAuditLog: id=%s action=%s target=%s", entry.ID, entry.Action, entry.TargetID)
	return nil
}

// ListAuditLogs returns a page of audit entries plus the total count.
func (r *adminRepository) ListAuditLogs(ctx context.Context, f AuditFilters) ([]*models.AdminAuditLog, int64, error) {
	where := ` WHERE 1=1`
	args := []interface{}{}
	argID := 1

	if f.Action != "" {
		where += fmt.Sprintf(` AND action = $%d`, argID)
		args = append(args, f.Action)
		argID++
	}
	if f.Actor != "" {
		where += fmt.Sprintf(` AND (actor_id ILIKE $%d OR actor_email ILIKE $%d)`, argID, argID)
		args = append(args, "%"+f.Actor+"%")
		argID++
	}

	countQuery := `SELECT COUNT(*) FROM admin_audit_logs` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	query := `SELECT id, actor_id, actor_email, action, target_type, target_id, reason, created_at
FROM admin_audit_logs` + where + ` ORDER BY created_at DESC` +
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argID, argID+1)
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*models.AdminAuditLog
	for rows.Next() {
		e := &models.AdminAuditLog{}
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorEmail, &e.Action, &e.TargetType, &e.TargetID, &e.Reason, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit log: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return entries, total, nil
}

// ListAuditByTarget returns the latest audit entries targeting a user.
func (r *adminRepository) ListAuditByTarget(ctx context.Context, targetID string, limit int) ([]*models.AdminAuditLog, error) {
	query := `SELECT id, actor_id, actor_email, action, target_type, target_id, reason, created_at
FROM admin_audit_logs
WHERE target_id = $1
ORDER BY created_at DESC
LIMIT $2`
	rows, err := r.db.QueryContext(ctx, query, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit by target: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var entries []*models.AdminAuditLog
	for rows.Next() {
		e := &models.AdminAuditLog{}
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorEmail, &e.Action, &e.TargetType, &e.TargetID, &e.Reason, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ListComments returns a page of all comments (moderation view) plus the
// total count for the filters. q matches content ILIKE with escaped
// wildcards; template_id/user_id are exact matches.
func (r *adminRepository) ListComments(ctx context.Context, f CommentFilters) ([]*models.AdminComment, int64, error) {
	where := ` WHERE 1=1`
	args := []interface{}{}
	argID := 1

	if f.Search != "" {
		escaped := strings.ReplaceAll(f.Search, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `%`, `\%`)
		escaped = strings.ReplaceAll(escaped, `_`, `\_`)
		where += fmt.Sprintf(` AND c.content ILIKE $%d ESCAPE '\'`, argID)
		args = append(args, "%"+escaped+"%")
		argID++
	}
	if f.TemplateID != "" {
		where += fmt.Sprintf(` AND c.template_id::text = $%d`, argID)
		args = append(args, f.TemplateID)
		argID++
	}
	if f.UserID != "" {
		where += fmt.Sprintf(` AND c.user_id = $%d`, argID)
		args = append(args, f.UserID)
		argID++
	}

	countQuery := `SELECT COUNT(*) FROM template_comments c` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count comments: %w", err)
	}

	query := `
SELECT c.id, c.template_id, t.title, t.owner_id, COALESCE(o.display_name, ''),
       c.user_id, COALESCE(u.display_name, ''), COALESCE(u.email, ''),
       c.content, c.created_at, c.updated_at
FROM template_comments c
JOIN templates t ON t.id = c.template_id
LEFT JOIN users o ON o.id = t.owner_id
LEFT JOIN users u ON u.id = c.user_id` + where + `
ORDER BY c.created_at DESC` + fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argID, argID+1)
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list comments: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var comments []*models.AdminComment
	for rows.Next() {
		c := &models.AdminComment{}
		if err := rows.Scan(
			&c.ID, &c.TemplateID, &c.TemplateTitle, &c.TemplateOwnerID, &c.OwnerDisplayName,
			&c.UserID, &c.UserDisplayName, &c.UserEmail,
			&c.Content, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return comments, total, nil
}

// DeleteComment removes any comment (admin moderation) and recomputes the
// template's comment_count in one transaction. Returns deleted=false when the
// comment does not exist.
func (r *adminRepository) DeleteComment(ctx context.Context, id string) (bool, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, fmt.Errorf("failed to begin delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var templateID string
	err = tx.QueryRowContext(ctx,
		`DELETE FROM template_comments WHERE id = $1 RETURNING template_id`, id,
	).Scan(&templateID)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to delete comment: %w", err)
	}

	var count int64
	if err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM template_comments WHERE template_id = $1`, templateID,
	).Scan(&count); err != nil {
		return false, 0, fmt.Errorf("failed to recount comments: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE templates SET comment_count = $1 WHERE id = $2`, count, templateID); err != nil {
		return false, 0, fmt.Errorf("failed to update comment_count: %w", err)
	}
	return true, count, tx.Commit()
}
