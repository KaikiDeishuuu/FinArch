-- V12: modern account deletion flow (one-time signed token tracking)
CREATE TABLE IF NOT EXISTS account_deletion_requests (
    jti        TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     TEXT NOT NULL CHECK(status IN ('pending','completed','expired')) DEFAULT 'pending',
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_account_delete_user ON account_deletion_requests(user_id, status);
CREATE INDEX IF NOT EXISTS idx_account_delete_exp ON account_deletion_requests(expires_at);
