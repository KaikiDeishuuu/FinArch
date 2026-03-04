-- V13: unified action request store for replay-safe security flows
CREATE TABLE IF NOT EXISTS action_requests (
    jti        TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action     TEXT NOT NULL,
    status     TEXT NOT NULL CHECK(status IN ('pending','completed','expired')) DEFAULT 'pending',
    meta       TEXT NOT NULL DEFAULT '',
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_action_requests_user_action ON action_requests(user_id, action, status);
CREATE INDEX IF NOT EXISTS idx_action_requests_exp ON action_requests(expires_at);
