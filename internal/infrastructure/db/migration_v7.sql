-- V7: username (unique, immutable) + pending_email + change_email token kind

-- Add username column; copy existing name values so legacy accounts are migrated.
ALTER TABLE users ADD COLUMN username TEXT NOT NULL DEFAULT '';
UPDATE users SET username = name WHERE username = '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username) WHERE deleted_at IS NULL;

-- pending_email stores the unverified new email during an email-change flow.
ALTER TABLE users ADD COLUMN pending_email TEXT;

-- Rebuild email_tokens to support 'change_email' kind and carry a meta payload
-- (used to store the new email address for change_email tokens).
CREATE TABLE IF NOT EXISTS email_tokens_v7 (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK(kind IN ('verify','reset','delete','change_email')),
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    meta       TEXT NOT NULL DEFAULT ''
);
INSERT INTO email_tokens_v7 (token, user_id, kind, expires_at, created_at)
    SELECT token, user_id, kind, expires_at, created_at FROM email_tokens;
DROP TABLE email_tokens;
ALTER TABLE email_tokens_v7 RENAME TO email_tokens;
CREATE INDEX IF NOT EXISTS idx_email_tokens_user ON email_tokens(user_id, kind);
