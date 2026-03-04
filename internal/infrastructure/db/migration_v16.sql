-- V16: Enterprise Concurrency Upgrade
-- Adds refresh_tokens, idempotency_keys, and global optimistic locking version columns.

CREATE TABLE IF NOT EXISTS refresh_tokens (
	id          TEXT PRIMARY KEY,
	user_id     TEXT NOT NULL,
	token_hash  TEXT NOT NULL UNIQUE,
	expires_at  DATETIME NOT NULL,
	consumed_at DATETIME NULL,
	created_at  DATETIME NOT NULL,
	version     INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
	id            TEXT PRIMARY KEY,
	user_id       TEXT NOT NULL,
	endpoint      TEXT NOT NULL,
	response_hash TEXT,
	created_at    DATETIME NOT NULL
);

-- Apply version column to all remaining mutable tables.
-- accounts table already received "version" in migration V9.

ALTER TABLE users ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE categories ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE transactions ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE reimbursements ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE tags ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE fund_pools ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE projects ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
