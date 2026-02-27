-- V2 migration: users, tags, transaction_tags, fund_pools
PRAGMA foreign_keys = ON;

-- ============================================================
-- users
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE COLLATE NOCASE,
    name         TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'owner',
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    deleted_at   INTEGER
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE deleted_at IS NULL;

-- ============================================================
-- tags
-- ============================================================
CREATE TABLE IF NOT EXISTS tags (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    name       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '#6366f1',
    created_at INTEGER NOT NULL,
    UNIQUE(owner_id, name)
);
CREATE INDEX IF NOT EXISTS idx_tags_owner ON tags(owner_id);

-- ============================================================
-- transaction_tags  (M:N)
-- ============================================================
CREATE TABLE IF NOT EXISTS transaction_tags (
    transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    tag_id         TEXT NOT NULL REFERENCES tags(id)          ON DELETE CASCADE,
    PRIMARY KEY (transaction_id, tag_id)
);
CREATE INDEX IF NOT EXISTS idx_tx_tags_tag ON transaction_tags(tag_id);

-- ============================================================
-- fund_pools  (多资金池)
-- ============================================================
CREATE TABLE IF NOT EXISTS fund_pools (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    project_id  TEXT REFERENCES projects(id),
    name        TEXT NOT NULL,
    pool_type   TEXT NOT NULL DEFAULT 'company',
    currency    TEXT NOT NULL DEFAULT 'CNY',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL,
    deleted_at  INTEGER
);
CREATE INDEX IF NOT EXISTS idx_fund_pools_owner ON fund_pools(owner_id) WHERE deleted_at IS NULL;

-- add owner_id + fund_pool_id to transactions (duplicate column errors are ignored by migration runner)
ALTER TABLE transactions ADD COLUMN owner_id TEXT REFERENCES users(id);
ALTER TABLE transactions ADD COLUMN fund_pool_id TEXT REFERENCES fund_pools(id);
ALTER TABLE transactions ADD COLUMN tags_cache TEXT NOT NULL DEFAULT '[]';

CREATE INDEX IF NOT EXISTS idx_tx_owner ON transactions(owner_id, occurred_at) WHERE owner_id IS NOT NULL;
