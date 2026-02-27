-- V4 migration: add user_id to transactions for per-user data isolation
ALTER TABLE transactions ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tx_user_occurred ON transactions(user_id, occurred_at DESC);
