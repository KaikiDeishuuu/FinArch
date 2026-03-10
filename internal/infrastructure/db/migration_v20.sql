-- V20: persisted conversion metadata for multi-currency accounting
ALTER TABLE transactions ADD COLUMN base_currency TEXT NOT NULL DEFAULT 'CNY';
ALTER TABLE transactions ADD COLUMN exchange_rate_source TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE transactions ADD COLUMN exchange_rate_at INTEGER;

UPDATE transactions
SET base_currency = COALESCE((SELECT currency FROM accounts WHERE accounts.id = transactions.account_id), 'CNY')
WHERE base_currency IS NULL OR base_currency = '' OR base_currency = 'CNY';

UPDATE transactions
SET exchange_rate_at = COALESCE(exchange_rate_at, transaction_time, CAST(strftime('%s','now') AS INTEGER))
WHERE exchange_rate_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tx_currency_pair_rate_time
ON transactions(user_id, currency, base_currency, exchange_rate_at DESC);
