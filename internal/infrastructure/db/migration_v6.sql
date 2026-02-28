-- V6: add 'delete' kind to email_tokens (account deletion verification)
-- SQLite does not support ALTER CONSTRAINT, so recreate the table.
CREATE TABLE IF NOT EXISTS email_tokens_new (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK(kind IN ('verify','reset','delete')),
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
INSERT INTO email_tokens_new SELECT * FROM email_tokens;
DROP TABLE email_tokens;
ALTER TABLE email_tokens_new RENAME TO email_tokens;
