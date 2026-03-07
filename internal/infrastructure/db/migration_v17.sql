-- V17: Financial Ledger Engine (journal + immutable ledger + snapshots)

PRAGMA foreign_keys = ON;

-- 1. Ledger accounts (chart of accounts per user)
CREATE TABLE IF NOT EXISTS ledger_accounts (
  id          TEXT    NOT NULL PRIMARY KEY,
  user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT    NOT NULL,
  type        TEXT    NOT NULL CHECK (type IN ('asset','liability','expense','income','equity')),
  currency    TEXT    NOT NULL DEFAULT 'CNY',
  status      TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 2. Journal entries (one logical financial event)
CREATE TABLE IF NOT EXISTS ledger_journal_entries (
  id             TEXT    NOT NULL PRIMARY KEY,
  user_id        TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reference_id   TEXT,
  description    TEXT    NOT NULL DEFAULT '',
  source         TEXT    NOT NULL DEFAULT 'transaction',
  status         TEXT    NOT NULL DEFAULT 'posted' CHECK (status IN ('draft','posted','void')),
  created_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  entry_hash     TEXT    NOT NULL,
  previous_hash  TEXT
);

-- 3. Journal lines (double-entry legs)
CREATE TABLE IF NOT EXISTS ledger_journal_lines (
  id          TEXT    NOT NULL PRIMARY KEY,
  entry_id    TEXT    NOT NULL REFERENCES ledger_journal_entries(id) ON DELETE CASCADE,
  account_id  TEXT    NOT NULL REFERENCES ledger_accounts(id) ON DELETE RESTRICT,
  debit_cents INTEGER NOT NULL DEFAULT 0 CHECK (debit_cents >= 0),
  credit_cents INTEGER NOT NULL DEFAULT 0 CHECK (credit_cents >= 0),
  currency    TEXT    NOT NULL DEFAULT 'CNY',
  created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 4. Event log for full auditability of ledger operations
CREATE TABLE IF NOT EXISTS ledger_events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type  TEXT    NOT NULL,
  entity_id   TEXT    NOT NULL,
  user_id     TEXT,
  payload_json TEXT,
  created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 5. Balance cache (derived from journal lines)
CREATE TABLE IF NOT EXISTS ledger_balance_cache (
  user_id       TEXT    NOT NULL,
  account_id    TEXT    NOT NULL,
  balance_cents INTEGER NOT NULL DEFAULT 0,
  updated_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (user_id, account_id),
  FOREIGN KEY (account_id) REFERENCES ledger_accounts(id) ON DELETE CASCADE
);

-- 6. Periodic snapshots for fast recovery and tamper detection
CREATE TABLE IF NOT EXISTS ledger_snapshots (
  id                 TEXT    NOT NULL PRIMARY KEY,
  user_id            TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  snapshot_balance_cents INTEGER NOT NULL,
  checksum           TEXT    NOT NULL,
  created_at         TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 7. Indexes for performance
CREATE INDEX IF NOT EXISTS idx_ledger_accounts_user
  ON ledger_accounts(user_id);

CREATE INDEX IF NOT EXISTS idx_ledger_entries_user_created
  ON ledger_journal_entries(user_id, created_at);

CREATE INDEX IF NOT EXISTS idx_ledger_lines_entry
  ON ledger_journal_lines(entry_id);

CREATE INDEX IF NOT EXISTS idx_ledger_lines_account
  ON ledger_journal_lines(account_id);

CREATE INDEX IF NOT EXISTS idx_ledger_events_user_type_time
  ON ledger_events(user_id, event_type, created_at);

CREATE INDEX IF NOT EXISTS idx_ledger_snapshots_user_time
  ON ledger_snapshots(user_id, created_at);

