-- V8: pwd_version counter for session invalidation on password change
--     Also fixes email_tokens kind constraint to include 'change_email_old'

ALTER TABLE users ADD COLUMN pwd_version INTEGER NOT NULL DEFAULT 0;

-- Rebuild email_tokens with updated kind constraint that includes change_email_old
CREATE TABLE IF NOT EXISTS email_tokens_v8 (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK(kind IN ('verify','reset','delete','change_email','change_email_old')),
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    meta       TEXT NOT NULL DEFAULT ''
);
INSERT INTO email_tokens_v8 SELECT token, user_id, kind, expires_at, created_at, meta FROM email_tokens;
DROP TABLE email_tokens;
ALTER TABLE email_tokens_v8 RENAME TO email_tokens;
CREATE INDEX IF NOT EXISTS idx_email_tokens_user ON email_tokens(user_id, kind)
