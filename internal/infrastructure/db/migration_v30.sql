-- V30: restore transaction hashes are audit/search metadata, not identity.
--
-- Two legitimate source transactions may have identical financial fields.
-- Cross-account restore idempotency is instead provided by the deterministic
-- target transaction ID derived from (source user, target user, source ID).
DROP INDEX IF EXISTS ux_txn_restore_hash_per_user;

CREATE INDEX IF NOT EXISTS idx_txn_restore_hash_per_user
ON transactions(user_id, restore_txn_hash)
WHERE restore_txn_hash IS NOT NULL AND restore_txn_hash <> '';
