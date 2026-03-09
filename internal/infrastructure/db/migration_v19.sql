-- V19: second-level transaction time + report/reimburse audit timestamps + stable ordering indexes
ALTER TABLE transactions ADD COLUMN transaction_time INTEGER;
ALTER TABLE transactions ADD COLUMN reported_at INTEGER;
ALTER TABLE transactions ADD COLUMN reimbursed_at INTEGER;

-- Backfill legacy rows from txn_date (YYYY-MM-DD -> 00:00:00 UTC)
UPDATE transactions
SET transaction_time = COALESCE(transaction_time, CAST(strftime('%s', txn_date || ' 00:00:00') AS INTEGER))
WHERE transaction_time IS NULL;

CREATE INDEX IF NOT EXISTS idx_transactions_time
ON transactions(transaction_time DESC);

CREATE INDEX IF NOT EXISTS idx_transactions_mode_time
ON transactions(mode, transaction_time DESC);

CREATE INDEX IF NOT EXISTS idx_transactions_reported_at
ON transactions(reported_at);

CREATE INDEX IF NOT EXISTS idx_transactions_reimbursed_at
ON transactions(reimbursed_at);
