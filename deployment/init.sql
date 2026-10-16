-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- -----------------------------------------------------------------------------
-- Table: users
-- Description: Stores user account information.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY CHECK (id ~ '^[a-zA-Z0-9_]+$'),
    email TEXT UNIQUE NOT NULL,
    mobile TEXT UNIQUE,
    password_hash TEXT NOT NULL, -- For local auth
    display_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE users IS 'Stores user account information';
COMMENT ON COLUMN users.id IS 'Unique user ID (alphanumeric + underscore)';
COMMENT ON COLUMN users.email IS 'Unique email address';
COMMENT ON COLUMN users.mobile IS 'Unique mobile number';
COMMENT ON COLUMN users.password_hash IS 'Bcrypt hash of the password';
COMMENT ON COLUMN users.display_name IS 'User display name';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='avatar') THEN
        ALTER TABLE users ADD COLUMN avatar TEXT;
        COMMENT ON COLUMN users.avatar IS 'User avatar URL or base64 string';
    END IF;
END $$;

-- -----------------------------------------------------------------------------
-- Table: user_identities
-- Description: Stores OAuth identities for users (SSO).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_identities (
    id SERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL, -- google, github, wechat
    provider_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_id)
);

COMMENT ON TABLE user_identities IS 'Stores OAuth identities for users';
COMMENT ON COLUMN user_identities.user_id IS 'Reference to the user';
COMMENT ON COLUMN user_identities.provider IS 'OAuth provider name (e.g., google, github)';
COMMENT ON COLUMN user_identities.provider_id IS 'Unique ID from the provider';

-- -----------------------------------------------------------------------------
-- Table: templates
-- Description: Stores the metadata for prompt templates.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    visibility TEXT NOT NULL CHECK (visibility IN ('public', 'private')),
    type TEXT NOT NULL CHECK (type IN ('system', 'user')),
    tags TEXT[], -- Array of strings for tags
    category TEXT,
    language TEXT NOT NULL DEFAULT 'en',
    like_count INT NOT NULL DEFAULT 0,
    favorite_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add comments for documentation
COMMENT ON TABLE templates IS 'Stores metadata for prompt templates';
COMMENT ON COLUMN templates.id IS 'Unique identifier for the template';
COMMENT ON COLUMN templates.owner_id IS 'ID of the user who owns the template';
COMMENT ON COLUMN templates.title IS 'Title of the template';
COMMENT ON COLUMN templates.visibility IS 'Visibility status: public or private';
COMMENT ON COLUMN templates.type IS 'Type of template: system or user';
COMMENT ON COLUMN templates.tags IS 'List of tags associated with the template';
COMMENT ON COLUMN templates.language IS 'Language of the template';
COMMENT ON COLUMN templates.like_count IS 'Number of likes';
COMMENT ON COLUMN templates.favorite_count IS 'Number of favorites';

-- Indexes for templates
CREATE INDEX IF NOT EXISTS idx_templates_owner_id ON templates(owner_id);
CREATE INDEX IF NOT EXISTS idx_templates_visibility ON templates(visibility);
CREATE INDEX IF NOT EXISTS idx_templates_category ON templates(category);
CREATE INDEX IF NOT EXISTS idx_templates_tags ON templates USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_templates_created_at ON templates(created_at);
CREATE INDEX IF NOT EXISTS idx_templates_language ON templates(language);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='templates' AND column_name='language') THEN
        ALTER TABLE templates ADD COLUMN language TEXT NOT NULL DEFAULT 'en';
        CREATE INDEX IF NOT EXISTS idx_templates_language ON templates(language);
    END IF;
END $$;

-- -----------------------------------------------------------------------------
-- Table: template_likes
-- Description: Stores user likes for templates.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_likes (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, template_id)
);

-- -----------------------------------------------------------------------------
-- Table: template_favorites
-- Description: Stores user favorites for templates.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_favorites (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, template_id)
);

-- -----------------------------------------------------------------------------
-- Table: template_versions
-- Description: Stores the actual content and version history of templates.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_versions (
    id SERIAL PRIMARY KEY,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    version INT NOT NULL, -- Logical version number (1, 2, 3...)
    content TEXT NOT NULL, -- The prompt content with $$ placeholders
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (template_id, version)
);

-- Add comments for documentation
COMMENT ON TABLE template_versions IS 'Stores content versions for templates';
COMMENT ON COLUMN template_versions.template_id IS 'Reference to the parent template';
COMMENT ON COLUMN template_versions.version IS 'Logical version number of the template';
COMMENT ON COLUMN template_versions.content IS 'The actual prompt text containing placeholders';

-- Indexes for template_versions
CREATE INDEX IF NOT EXISTS idx_template_versions_template_id ON template_versions(template_id);

-- -----------------------------------------------------------------------------
-- Table: prompts
-- Description: Stores instantiated prompts created by users from templates.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS prompts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    version_id INT NOT NULL REFERENCES template_versions(id) ON DELETE CASCADE,
    owner_id TEXT NOT NULL, -- User who created/saved this prompt instance
    variables JSONB NOT NULL, -- List of strings used to replace placeholders
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add comments for documentation
COMMENT ON TABLE prompts IS 'Stores instantiated prompts saved by users';
COMMENT ON COLUMN prompts.template_id IS 'Reference to the template used';
COMMENT ON COLUMN prompts.version_id IS 'Reference to the specific version of the template used';
COMMENT ON COLUMN prompts.variables IS 'JSON array of strings replacing the placeholders';
COMMENT ON COLUMN prompts.owner_id IS 'ID of the user who saved this prompt';

-- Indexes for prompts
CREATE INDEX IF NOT EXISTS idx_prompts_owner_id ON prompts(owner_id);
CREATE INDEX IF NOT EXISTS idx_prompts_template_id ON prompts(template_id);

-- -----------------------------------------------------------------------------
-- Table: template_aliases
-- Description: Stores aliases for template versions.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_aliases (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    alias_name VARCHAR(50) NOT NULL,
    version_id INT NOT NULL REFERENCES template_versions(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(template_id, alias_name)
);

-- -----------------------------------------------------------------------------
-- Table: api_keys
-- Description: Stores API keys for user authentication.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL, -- Store hash of the key, not the key itself
    prefix TEXT NOT NULL, -- partial key for display
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(key_hash)
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
COMMENT ON TABLE api_keys IS 'Stores API keys for user authentication';

-- -----------------------------------------------------------------------------
-- Admin: users.role / users.status
-- Description: Role-based access control and account status for administration.
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='role') THEN
        ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';
        COMMENT ON COLUMN users.role IS 'Account role: user or admin';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='status') THEN
        ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
        COMMENT ON COLUMN users.status IS 'Account status: active or banned';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);

-- -----------------------------------------------------------------------------
-- Table: admin_audit_logs
-- Description: Append-only audit trail of administrative mutations.
--              Deliberately no FK: history must survive user deletion.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_id TEXT NOT NULL,
    actor_email TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('ban', 'unban', 'role_change')),
    target_type TEXT NOT NULL CHECK (target_type IN ('user')),
    target_id TEXT NOT NULL,
    reason TEXT NOT NULL CHECK (char_length(reason) BETWEEN 3 AND 500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE admin_audit_logs IS 'Append-only audit trail of admin mutations';
COMMENT ON COLUMN admin_audit_logs.actor_id IS 'Acting admin user ID (from verified JWT)';
COMMENT ON COLUMN admin_audit_logs.actor_email IS 'Admin email snapshot at action time';
COMMENT ON COLUMN admin_audit_logs.action IS 'ban | unban | role_change';
COMMENT ON COLUMN admin_audit_logs.target_type IS 'Target entity type (user)';
COMMENT ON COLUMN admin_audit_logs.target_id IS 'Affected user ID';
COMMENT ON COLUMN admin_audit_logs.reason IS 'Mandatory justification, 3-500 chars';

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_created_at ON admin_audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_actor_id ON admin_audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_target_id ON admin_audit_logs(target_id);

-- -----------------------------------------------------------------------------
-- Table: notifications
-- Description: Engagement notifications (like/favorite/fork) for template
--              owners. Recipient is always the template owner; actor is the
--              engaging user. Self-actions are suppressed in the service
--              layer and enforced by the no_self_notify CHECK as a backstop.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('like', 'favorite', 'fork')),
    actor_display_name TEXT NOT NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT no_self_notify CHECK (recipient_id <> actor_id)
);

COMMENT ON TABLE notifications IS 'Engagement notifications for template owners';
COMMENT ON COLUMN notifications.recipient_id IS 'Template owner receiving the notification';
COMMENT ON COLUMN notifications.actor_id IS 'User who liked/favorited/forked the template';
COMMENT ON COLUMN notifications.template_id IS 'Template the event happened on (cascade-deleted with it)';
COMMENT ON COLUMN notifications.type IS 'Event type: like | favorite | fork';
COMMENT ON COLUMN notifications.actor_display_name IS 'Actor display name snapshot at event time';
COMMENT ON COLUMN notifications.read_at IS 'NULL while unread';

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_unread
    ON notifications (recipient_id) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_created
    ON notifications (recipient_id, created_at DESC);

-- -----------------------------------------------------------------------------
-- Feature: template-comments
-- Description: Flat comment threads on templates. comment_count is maintained
--              transactionally on insert/delete (like like_count).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_comments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE template_comments IS 'Flat comments on templates (no threading)';
COMMENT ON COLUMN template_comments.content IS 'Plain text, 1-2000 chars, rendered client-side with escaping only';

CREATE INDEX IF NOT EXISTS idx_template_comments_template_created
    ON template_comments (template_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_template_comments_user_id ON template_comments (user_id);

-- Denormalized counter (same pattern as like_count / favorite_count).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='comment_count') THEN
        ALTER TABLE templates ADD COLUMN comment_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN templates.comment_count IS 'Number of comments (maintained transactionally)';
    END IF;
END $$;

-- Widen admin_audit_logs CHECKs for delete_comment / comment targets.
-- CHECK constraints cannot be ALTERed; guard on pg_constraint so the block is
-- idempotent: only drop+re-add when the live definition lacks the new value.
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%delete_comment%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment'));
    END IF;
END $$;

DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_target_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%comment%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_target_type_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_target_type_check
            CHECK (target_type IN ('user', 'comment'));
    END IF;
END $$;

-- Widen notifications.type CHECK for the 'comment' engagement type
-- (notifications landed first; same pg_constraint-guarded pattern).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%comment%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment'));
    END IF;
END $$;

-- -----------------------------------------------------------------------------
-- Template versioning UX: change notes + prompt usage counts
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='template_versions' AND column_name='change_note') THEN
        ALTER TABLE template_versions ADD COLUMN change_note TEXT;
        COMMENT ON COLUMN template_versions.change_note IS 'Optional user-provided change note (max 200 chars, enforced in service layer)';
    END IF;
END $$;

-- Supports the prompt_count LEFT JOIN in ListTemplateVersions
CREATE INDEX IF NOT EXISTS idx_prompts_version_id ON prompts(version_id);

-- -----------------------------------------------------------------------------
-- Template search & filtering: performance indexes for trending sort
-- (7-day engagement window derived-table joins).
-- -----------------------------------------------------------------------------
CREATE INDEX IF NOT EXISTS idx_template_likes_template_created
    ON template_likes(template_id, created_at);
CREATE INDEX IF NOT EXISTS idx_template_favorites_template_created
    ON template_favorites(template_id, created_at);

-- -----------------------------------------------------------------------------
-- Feature: pre-publication-review
-- Description: gating state, review submissions, platform settings.
-- -----------------------------------------------------------------------------

-- Gating state on users (OQ-1: denormalized counters). The backfill runs
-- inside the column-add guard, so it executes exactly once — on first
-- migration onto this schema.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='approved_template_count') THEN
        ALTER TABLE users ADD COLUMN approved_template_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.approved_template_count IS
            'Approved templates maintained at review decision time (gating threshold N)';
        -- Backfill: existing public templates count as approved.
        UPDATE users u
        SET approved_template_count = (
            SELECT COUNT(*) FROM templates t
            WHERE t.owner_id = u.id AND t.visibility = 'public'
        );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='review_gated_remaining') THEN
        ALTER TABLE users ADD COLUMN review_gated_remaining INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.review_gated_remaining IS
            'Publishes still gated after a moderation takedown/ban (re-gate count M)';
    END IF;
END $$;

-- Moderation state + first-approval time on templates. The published_at
-- backfill runs inside the column-add guard (once, on first migration).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='moderation_status') THEN
        ALTER TABLE templates ADD COLUMN moderation_status TEXT NOT NULL DEFAULT 'normal';
        ALTER TABLE templates ADD CONSTRAINT templates_moderation_status_check
            CHECK (moderation_status IN ('normal', 'in_review', 'rejected'));
        COMMENT ON COLUMN templates.moderation_status IS
            'normal | in_review | rejected (admin-template-moderation widens this list later)';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='published_at') THEN
        ALTER TABLE templates ADD COLUMN published_at TIMESTAMPTZ;
        COMMENT ON COLUMN templates.published_at IS 'First review-approval time; NULL until approved';
        -- Backfill: existing public templates are considered published at creation.
        UPDATE templates SET published_at = created_at
        WHERE visibility = 'public' AND published_at IS NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_templates_moderation_status
    ON templates(moderation_status);

-- Review submissions (OQ-2: separate table; chain via UNIQUE(template_id, attempt_number)).
CREATE TABLE IF NOT EXISTS review_submissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    creator_id TEXT NOT NULL,
    attempt_number INT NOT NULL CHECK (attempt_number >= 1),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at TIMESTAMPTZ,
    decided_by TEXT,
    reject_reason_code TEXT,
    reject_note TEXT,
    UNIQUE (template_id, attempt_number)
);

COMMENT ON TABLE review_submissions IS 'Pre-publication review queue; one row per publish attempt';
COMMENT ON COLUMN review_submissions.attempt_number IS '1 = first submission; >= 2 = resubmission';
COMMENT ON COLUMN review_submissions.status IS 'pending | approved | rejected';
COMMENT ON COLUMN review_submissions.reject_reason_code IS 'spam | plagiarism | harmful | lowQuality | other (NULL for approvals)';

-- Partial index serves the queue: pending only, aging-first + FIFO order.
CREATE INDEX IF NOT EXISTS idx_review_submissions_pending
    ON review_submissions (status, submitted_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_review_submissions_template
    ON review_submissions (template_id);
CREATE INDEX IF NOT EXISTS idx_review_submissions_creator
    ON review_submissions (creator_id, status);

-- Platform settings (NFR-5: N/M/aging configurable without deploy).
CREATE TABLE IF NOT EXISTS platform_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE platform_settings IS 'Admin-editable platform settings (key-value)';

INSERT INTO platform_settings (key, value) VALUES
    ('review_approval_threshold', '3'),
    ('review_regate_count', '3'),
    ('review_aging_hours', '48')
ON CONFLICT (key) DO NOTHING;

-- Widen admin_audit_logs CHECKs for review decisions (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%review_approve%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change'));
    END IF;
END $$;

DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_target_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_target_type_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_target_type_check
            CHECK (target_type IN ('user', 'comment', 'template', 'platform'));
    END IF;
END $$;

-- Widen notifications.type CHECK for decision notifications (guarded).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template_approved%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment', 'template_approved', 'template_rejected'));
    END IF;
END $$;

-- -----------------------------------------------------------------------------
-- Feature: content-warnings
-- Description: config-driven warning taxonomy, per-template warning arrays,
--              moderator lock, audit + notification widenings.
-- -----------------------------------------------------------------------------

-- Viewer preference: skip the blur/interstitial for warned templates
-- (FR-5). Default off; additive, no backfill needed.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='show_warned_content') THEN
        ALTER TABLE users ADD COLUMN show_warned_content BOOLEAN NOT NULL DEFAULT false;
        COMMENT ON COLUMN users.show_warned_content IS
            'Viewer preference: when true, skip the blur/interstitial for warned templates';
    END IF;
END $$;

-- Warning arrays on templates (OQ-1: two parallel TEXT[] columns; the lock is
-- the intersection). Default empty — no backfill needed for existing rows.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='content_warnings') THEN
        ALTER TABLE templates ADD COLUMN content_warnings TEXT[] NOT NULL DEFAULT '{}';
        COMMENT ON COLUMN templates.content_warnings IS
            'Active content warnings (self-declared + moderator-applied), keys from the config-driven taxonomy';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='locked_warnings') THEN
        ALTER TABLE templates ADD COLUMN locked_warnings TEXT[] NOT NULL DEFAULT '{}';
        COMMENT ON COLUMN templates.locked_warnings IS
            'Subset of content_warnings applied by moderation; creator-immutable (FR-8)';
    END IF;
END $$;

-- GIN index serves exclude_warned on public lists and the admin warning-state
-- filter (NFR-6).
CREATE INDEX IF NOT EXISTS idx_templates_content_warnings
    ON templates USING GIN(content_warnings);

-- Taxonomy seed (NFR-2: config-driven, admin-editable without deploy).
INSERT INTO platform_settings (key, value) VALUES
    ('content_warning_taxonomy', '["mature","violence","profanity","sensitive"]')
ON CONFLICT (key) DO NOTHING;

-- Widen admin_audit_logs.action CHECK for warning changes (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%template.warning_add%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove'));
    END IF;
END $$;

-- Widen notifications.type CHECK for warning-change notifications (guarded).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template_warning_changed%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed'));
    END IF;
END $$;

-- -----------------------------------------------------------------------------
-- Feature: follow-creators
-- Description: follow graph, denormalized counters, bio, and the 'follow'
--              notification event (template_id nullable).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_follows (
    follower_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CONSTRAINT chk_no_self_follow CHECK (follower_id <> followee_id)
);

COMMENT ON TABLE user_follows IS 'Directed follow graph (follower -> followee)';
COMMENT ON COLUMN user_follows.follower_id IS 'User doing the following';
COMMENT ON COLUMN user_follows.followee_id IS 'User being followed';

CREATE INDEX IF NOT EXISTS idx_user_follows_followee
    ON user_follows(followee_id, created_at DESC);

-- Denormalized counters (same pattern as like_count / favorite_count).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='bio') THEN
        ALTER TABLE users ADD COLUMN bio TEXT;
        COMMENT ON COLUMN users.bio IS 'Optional profile bio, 0-280 chars (empty clears)';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='follower_count') THEN
        ALTER TABLE users ADD COLUMN follower_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.follower_count IS 'Number of followers (maintained transactionally)';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='following_count') THEN
        ALTER TABLE users ADD COLUMN following_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.following_count IS 'Number of users this user follows (maintained transactionally)';
    END IF;
END $$;

-- 'follow' notifications carry no template: make template_id nullable
-- (guarded, idempotent, order-independent vs. the notifications feature).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'notifications')
       AND EXISTS (
           SELECT 1 FROM pg_attribute
           WHERE attrelid = 'notifications'::regclass
             AND attname = 'template_id' AND attnotnull
       ) THEN
        ALTER TABLE notifications ALTER COLUMN template_id DROP NOT NULL;
    END IF;
END $$;

-- Widen the notification type CHECK to include 'follow' (guarded drop/re-add,
-- same pattern as prior CHECK widenings; safe in any landing order).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%follow%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed', 'follow'));
    END IF;
END $$;

-- -----------------------------------------------------------------------------
-- Feature: prompt-analytics
-- Description: per-template view/use events, denormalized counters, and the
--              historical use backfill. view_count has no historical source
--              and starts at 0 (accepted in the architecture doc).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_usage_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('view', 'use')),
    source TEXT NOT NULL CHECK (source IN ('web', 'api')),
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    session_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tue_template_created
    ON template_usage_events(template_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tue_user_created
    ON template_usage_events(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tue_view_dedup
    ON template_usage_events(template_id, session_id, created_at)
    WHERE event_type = 'view';

COMMENT ON TABLE template_usage_events IS 'Per-template view/use events for analytics';
COMMENT ON COLUMN template_usage_events.event_type IS 'view | use';
COMMENT ON COLUMN template_usage_events.source IS 'web | api (origin of the event)';
COMMENT ON COLUMN template_usage_events.session_id IS 'Client session id (view dedup); NULL for use events';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'templates' AND column_name = 'view_count') THEN
        ALTER TABLE templates ADD COLUMN view_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN templates.view_count IS 'Denormalized view count (async maintained)';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'templates' AND column_name = 'use_count') THEN
        ALTER TABLE templates ADD COLUMN use_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN templates.use_count IS 'Denormalized use count (async maintained)';
    END IF;
END $$;

-- Backfill (D-2): one historical use/web event per prompts row. The
-- per-row NOT EXISTS guard makes re-runs and coexistence with live events
-- safe. Deploy DDL before the new binary so the guard imports history once.
INSERT INTO template_usage_events (template_id, event_type, source, user_id, created_at)
SELECT p.template_id, 'use', 'web', p.owner_id, p.created_at
FROM prompts p
WHERE NOT EXISTS (
    SELECT 1 FROM template_usage_events e
    WHERE e.event_type = 'use' AND e.template_id = p.template_id
      AND e.user_id IS NOT DISTINCT FROM p.owner_id
      AND e.created_at = p.created_at
);

-- Converge the denormalized counter from the event table (idempotent).
UPDATE templates t
SET use_count = sub.c
FROM (SELECT template_id, COUNT(*) AS c
      FROM template_usage_events WHERE event_type = 'use'
      GROUP BY template_id) sub
WHERE t.id = sub.template_id AND t.use_count <> sub.c;

-- ============================================
-- Template Collections
-- ============================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables
                   WHERE table_name = 'collections') THEN
        CREATE TABLE collections (
            id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
            owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            name TEXT NOT NULL,
            description TEXT,
            visibility TEXT NOT NULL CHECK (visibility IN ('private','public')) DEFAULT 'private',
            status TEXT NOT NULL CHECK (status IN ('active','deleted')) DEFAULT 'active',
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables
                   WHERE table_name = 'collection_templates') THEN
        CREATE TABLE collection_templates (
            collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
            template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
            position INT NOT NULL,
            added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            PRIMARY KEY (collection_id, template_id)
        );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_indexes
                   WHERE indexname = 'idx_collection_templates_position') THEN
        CREATE INDEX idx_collection_templates_position
            ON collection_templates (collection_id, position);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_indexes
                   WHERE indexname = 'idx_collections_owner') THEN
        CREATE INDEX idx_collections_owner
            ON collections (owner_id) WHERE status = 'active';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_indexes
                   WHERE indexname = 'idx_collections_public') THEN
        CREATE INDEX idx_collections_public
            ON collections (visibility, created_at DESC) WHERE status = 'active' AND visibility = 'public';
    END IF;
END $$;
