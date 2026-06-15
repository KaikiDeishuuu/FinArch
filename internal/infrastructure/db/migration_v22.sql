-- V22: read-path indexes for transaction lists and statistics.
CREATE INDEX IF NOT EXISTS idx_txn_user_mode_time
ON transactions(user_id, mode, transaction_time DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_txn_user_type_date
ON transactions(user_id, type, txn_date);

CREATE INDEX IF NOT EXISTS idx_txn_user_category_date
ON transactions(user_id, category, txn_date)
WHERE type = 'expense';

CREATE INDEX IF NOT EXISTS idx_txn_user_project_type
ON transactions(user_id, project_id, type)
WHERE project_id IS NOT NULL AND project_id != '';

CREATE INDEX IF NOT EXISTS idx_txn_user_pending_reimb
ON transactions(user_id, base_amount_cents)
WHERE reimb_status = 'pending';
