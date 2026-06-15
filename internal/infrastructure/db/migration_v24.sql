-- V24: recurring transactions, attachments, and OCR metadata

CREATE TABLE IF NOT EXISTS recurring_transaction_rules (
  id                  TEXT    NOT NULL PRIMARY KEY,
  user_id             TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mode                TEXT    NOT NULL CHECK(mode IN ('work','life')),
  name                TEXT    NOT NULL,
  status              TEXT    NOT NULL DEFAULT 'active' CHECK(status IN ('active','paused','ended')),
  account_id          TEXT    NOT NULL REFERENCES accounts(id),
  type                TEXT    NOT NULL CHECK(type IN ('income','expense')),
  category            TEXT    NOT NULL,
  amount_cents        INTEGER NOT NULL CHECK(amount_cents > 0),
  currency            TEXT    NOT NULL DEFAULT 'CNY',
  exchange_rate       REAL    NOT NULL DEFAULT 0,
  note                TEXT    NOT NULL DEFAULT '',
  project_id          TEXT    REFERENCES projects(id) ON DELETE SET NULL,
  frequency           TEXT    NOT NULL CHECK(frequency IN ('daily','weekly','monthly','yearly')),
  interval            INTEGER NOT NULL DEFAULT 1 CHECK(interval > 0),
  start_date          TEXT    NOT NULL CHECK(length(start_date) = 10),
  end_date            TEXT,
  time_of_day         TEXT    NOT NULL DEFAULT '09:00:00',
  timezone            TEXT    NOT NULL DEFAULT 'Local',
  day_of_week         INTEGER,
  day_of_month        INTEGER,
  month_end_policy    TEXT    NOT NULL DEFAULT 'clamp' CHECK(month_end_policy IN ('clamp','skip')),
  next_run_at         INTEGER NOT NULL,
  last_generated_for  TEXT,
  catch_up_enabled    INTEGER NOT NULL DEFAULT 1 CHECK(catch_up_enabled IN (0,1)),
  created_at          TEXT    NOT NULL,
  updated_at          TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recurring_rules_due
ON recurring_transaction_rules(user_id, mode, status, next_run_at);

CREATE INDEX IF NOT EXISTS idx_recurring_rules_user_created
ON recurring_transaction_rules(user_id, mode, created_at DESC);

CREATE TABLE IF NOT EXISTS recurring_transaction_instances (
  id                TEXT    NOT NULL PRIMARY KEY,
  rule_id           TEXT    NOT NULL REFERENCES recurring_transaction_rules(id) ON DELETE CASCADE,
  user_id           TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  occurrence_date   TEXT    NOT NULL,
  scheduled_at      INTEGER NOT NULL,
  transaction_id    TEXT    REFERENCES transactions(id) ON DELETE SET NULL,
  idempotency_key   TEXT    NOT NULL UNIQUE,
  status            TEXT    NOT NULL CHECK(status IN ('generating','generated','skipped','failed')),
  error             TEXT,
  created_at        TEXT    NOT NULL,
  updated_at        TEXT    NOT NULL,
  UNIQUE(rule_id, occurrence_date)
);

CREATE INDEX IF NOT EXISTS idx_recurring_instances_user_status
ON recurring_transaction_instances(user_id, status, scheduled_at);

CREATE INDEX IF NOT EXISTS idx_recurring_instances_rule_time
ON recurring_transaction_instances(rule_id, scheduled_at DESC);

CREATE INDEX IF NOT EXISTS idx_recurring_instances_transaction
ON recurring_transaction_instances(transaction_id);

ALTER TABLE transactions ADD COLUMN recurring_rule_id TEXT;
ALTER TABLE transactions ADD COLUMN recurring_occurrence_date TEXT;

CREATE INDEX IF NOT EXISTS idx_txn_recurring_rule
ON transactions(recurring_rule_id, recurring_occurrence_date);

CREATE TABLE IF NOT EXISTS attachments (
  id                 TEXT    NOT NULL PRIMARY KEY,
  user_id            TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  transaction_id     TEXT    REFERENCES transactions(id) ON DELETE CASCADE,
  storage_key        TEXT    NOT NULL UNIQUE,
  original_filename  TEXT    NOT NULL,
  content_type       TEXT    NOT NULL,
  size_bytes         INTEGER NOT NULL CHECK(size_bytes > 0),
  sha256             TEXT    NOT NULL,
  kind               TEXT    NOT NULL DEFAULT 'receipt' CHECK(kind IN ('receipt','invoice','other')),
  ocr_status         TEXT    NOT NULL DEFAULT 'not_requested' CHECK(ocr_status IN ('not_requested','pending','processing','done','failed','unavailable')),
  ocr_provider       TEXT,
  ocr_text           TEXT,
  ocr_json           TEXT,
  ocr_error          TEXT,
  created_at         TEXT    NOT NULL,
  updated_at         TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attachments_transaction
ON attachments(user_id, transaction_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_attachments_ocr_status
ON attachments(user_id, ocr_status, created_at);

CREATE INDEX IF NOT EXISTS idx_attachments_sha256
ON attachments(sha256);
