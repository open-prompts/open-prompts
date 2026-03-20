package repository

import (
"context"
"database/sql"
"fmt"
"time"

"open-prompts/backend/internal/models"
)

type TemplateAliasRepository interface {
Create(ctx context.Context, alias *models.TemplateAlias) error
Get(ctx context.Context, templateID, aliasName string) (*models.TemplateAlias, error)
List(ctx context.Context, templateID string) ([]*models.TemplateAlias, error)
Update(ctx context.Context, alias *models.TemplateAlias) error
Delete(ctx context.Context, templateID, aliasName string) error
Upsert(ctx context.Context, alias *models.TemplateAlias) error
}

type templateAliasRepository struct {
db *sql.DB
}

func NewTemplateAliasRepository(db *sql.DB) TemplateAliasRepository {
return &templateAliasRepository{db: db}
}

func (r *templateAliasRepository) Create(ctx context.Context, t *models.TemplateAlias) error {
query := `
INSERT INTO template_aliases (
template_id, alias_name, version_id, created_at, updated_at
) VALUES (
$1, $2, $3, $4, $5
) RETURNING id
`
t.CreatedAt = time.Now()
t.UpdatedAt = time.Now()
err := r.db.QueryRowContext(ctx, query, t.TemplateID, t.AliasName, t.VersionID, t.CreatedAt, t.UpdatedAt).Scan(&t.ID)
if err != nil {
return fmt.Errorf("failed to create alias: %w", err)
}
return nil
}

func (r *templateAliasRepository) Update(ctx context.Context, t *models.TemplateAlias) error {
query := `
UPDATE template_aliases
SET version_id = $1, updated_at = $2
WHERE template_id = $3 AND alias_name = $4
`
t.UpdatedAt = time.Now()
_, err := r.db.ExecContext(ctx, query, t.VersionID, t.UpdatedAt, t.TemplateID, t.AliasName)
if err != nil {
return fmt.Errorf("failed to update alias: %w", err)
}
return nil
}

func (r *templateAliasRepository) Upsert(ctx context.Context, t *models.TemplateAlias) error {
query := `
INSERT INTO template_aliases (template_id, alias_name, version_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (template_id, alias_name) DO UPDATE
SET version_id = EXCLUDED.version_id, updated_at = EXCLUDED.updated_at
RETURNING id
`
t.CreatedAt = time.Now()
t.UpdatedAt = time.Now()
err := r.db.QueryRowContext(ctx, query, t.TemplateID, t.AliasName, t.VersionID, t.CreatedAt, t.UpdatedAt).Scan(&t.ID)
if err != nil {
return fmt.Errorf("failed to upsert alias: %w", err)
}
return nil
}

func (r *templateAliasRepository) Delete(ctx context.Context, templateID, aliasName string) error {
query := `DELETE FROM template_aliases WHERE template_id = $1 AND alias_name = $2`
_, err := r.db.ExecContext(ctx, query, templateID, aliasName)
if err != nil {
return fmt.Errorf("failed to delete alias: %w", err)
}
return nil
}

func (r *templateAliasRepository) Get(ctx context.Context, templateID, aliasName string) (*models.TemplateAlias, error) {
query := `SELECT id, template_id, alias_name, version_id, created_at, updated_at FROM template_aliases WHERE template_id = $1 AND alias_name = $2`
var t models.TemplateAlias
err := r.db.QueryRowContext(ctx, query, templateID, aliasName).Scan(
&t.ID, &t.TemplateID, &t.AliasName, &t.VersionID, &t.CreatedAt, &t.UpdatedAt,
)
if err != nil {
if err == sql.ErrNoRows {
return nil, fmt.Errorf("alias not found")
}
return nil, fmt.Errorf("failed to get alias: %w", err)
}
return &t, nil
}

func (r *templateAliasRepository) List(ctx context.Context, templateID string) ([]*models.TemplateAlias, error) {
query := `SELECT id, template_id, alias_name, version_id, created_at, updated_at FROM template_aliases WHERE template_id = $1 ORDER BY updated_at DESC`
rows, err := r.db.QueryContext(ctx, query, templateID)
if err != nil {
return nil, fmt.Errorf("failed to list aliases: %w", err)
}
defer func() { _ = rows.Close() }()

var aliases []*models.TemplateAlias
for rows.Next() {
var t models.TemplateAlias
if err := rows.Scan(
&t.ID, &t.TemplateID, &t.AliasName, &t.VersionID, &t.CreatedAt, &t.UpdatedAt,
); err != nil {
return nil, fmt.Errorf("failed to scan alias: %w", err)
}
aliases = append(aliases, &t)
}
if err := rows.Err(); err != nil {
return nil, fmt.Errorf("rows error: %w", err)
}
return aliases, nil
}
