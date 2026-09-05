-- V27: revocable refresh-session families with replay-resistant rotation.
-- Legacy v16 refresh rows cannot be safely upgraded because they have no
-- family, credential-version, or absolute-expiry authority. Invalidate them.
DROP TABLE IF EXISTS refresh_tokens;

CREATE TABLE auth_sessions (
  id                  TEXT    NOT NULL PRIMARY KEY,
  user_id             TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  pwd_version         INTEGER NOT NULL CHECK(pwd_version >= 0),
  absolute_expires_at INTEGER NOT NULL,
  revoked_at          INTEGER,
  revoke_reason       TEXT,
  created_at          INTEGER NOT NULL,
  last_used_at        INTEGER NOT NULL,
  CHECK(absolute_expires_at > created_at)
);

CREATE INDEX idx_auth_sessions_user_active
ON auth_sessions(user_id, absolute_expires_at)
WHERE revoked_at IS NULL;

CREATE TABLE refresh_tokens (
  id                    TEXT    NOT NULL PRIMARY KEY,
  session_id            TEXT    NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
  token_hash            BLOB    NOT NULL UNIQUE CHECK(length(token_hash) = 32),
  generation            INTEGER NOT NULL CHECK(generation >= 0),
  expires_at            INTEGER NOT NULL,
  consumed_at           INTEGER,
  replaced_by_hash      BLOB    CHECK(replaced_by_hash IS NULL OR length(replaced_by_hash) = 32),
  retry_until           INTEGER,
  recovery_ciphertext   BLOB,
  created_at            INTEGER NOT NULL,
  UNIQUE(session_id, generation),
  CHECK(expires_at > created_at)
);

CREATE INDEX idx_refresh_tokens_session
ON refresh_tokens(session_id, generation);

CREATE INDEX idx_refresh_tokens_expiry
ON refresh_tokens(expires_at);
