-- V31: durable commit ledger for restore operations.
--
-- Cross-account restores use this table to reconcile attachment writes with
-- the importing SQL transaction. Replacement restores record their decision
-- in the newly validated database before publishing the external committed
-- marker, so recovery can resolve an ambiguous marker-directory fsync.
CREATE TABLE IF NOT EXISTS restore_operations (
  operation_id TEXT PRIMARY KEY,
  batch_id TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL CHECK(kind IN ('cross_merge', 'replace')),
  created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_restore_operations_kind_created
ON restore_operations(kind, created_at);
