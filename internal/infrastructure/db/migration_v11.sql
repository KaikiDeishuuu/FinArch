ALTER TABLE transactions ADD COLUMN mode TEXT NOT NULL DEFAULT 'work' CHECK(mode IN ('work','life'));
CREATE INDEX IF NOT EXISTS idx_txn_user_mode_date ON transactions(user_id, mode, txn_date DESC);
