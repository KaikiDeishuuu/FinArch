-- V18: restore metadata + DB-level dedup guard for recovered transactions
ALTER TABLE transactions ADD COLUMN restore_source_backup_id TEXT;
ALTER TABLE transactions ADD COLUMN restore_import_batch_id TEXT;
ALTER TABLE transactions ADD COLUMN restore_recovered_at TEXT;
ALTER TABLE transactions ADD COLUMN restore_txn_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_txn_restore_batch ON transactions(user_id, restore_import_batch_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_txn_restore_hash_per_user
ON transactions(user_id, restore_txn_hash)
WHERE restore_txn_hash IS NOT NULL AND restore_txn_hash <> '';
