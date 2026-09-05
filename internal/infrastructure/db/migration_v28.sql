-- V28: optimistic concurrency for recurring rule edits and generation.
ALTER TABLE recurring_transaction_rules
ADD COLUMN version INTEGER NOT NULL DEFAULT 1 CHECK(version > 0);
