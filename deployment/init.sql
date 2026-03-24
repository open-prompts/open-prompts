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
