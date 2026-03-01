-- V9: Production-grade double-entry schema
-- NOTE: triggers are in triggers_v9.go (ApplyTriggers) because CREATE TRIGGER
--       bodies contain semicolons that confuse the statement splitter.

PRAGMA foreign_keys = OFF;

-- 1. accounts
CREATE TABLE IF NOT EXISTS accounts (
  id             TEXT    NOT NULL PRIMARY KEY,
  user_id        TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name           TEXT    NOT NULL,
  type           TEXT    NOT NULL CHECK(type IN ('personal','public')),
  currency       TEXT    NOT NULL DEFAULT 'CNY',
  balance_cents  INTEGER NOT NULL DEFAULT 0,
  version        INTEGER NOT NULL DEFAULT 0,
  is_active      INTEGER NOT NULL DEFAULT 1 CHECK(is_active IN (0,1)),
  created_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 2. categories
CREATE TABLE IF NOT EXISTS categories (
  id         TEXT    NOT NULL PRIMARY KEY,
  user_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  type       TEXT    NOT NULL CHECK(type IN ('income','expense','transfer')),
  parent_id  TEXT    REFERENCES categories(id) ON DELETE SET NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_active  INTEGER NOT NULL DEFAULT 1,
  UNIQUE(user_id, name, type)
);

-- 3. audit_log
CREATE TABLE IF NOT EXISTS audit_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    TEXT,
  table_name TEXT    NOT NULL,
  row_id     TEXT    NOT NULL,
  action     TEXT    NOT NULL CHECK(action IN ('INSERT','UPDATE','DELETE')),
  old_data   TEXT,
  new_data   TEXT,
  ip_addr    TEXT,
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 4. monthly_summary_cache
CREATE TABLE IF NOT EXISTS monthly_summary_cache (
  user_id       TEXT    NOT NULL,
  account_id    TEXT    NOT NULL,
  month         TEXT    NOT NULL,
  income_cents  INTEGER NOT NULL DEFAULT 0,
  expense_cents INTEGER NOT NULL DEFAULT 0,
  refreshed_at  TEXT    NOT NULL,
  PRIMARY KEY (user_id, account_id, month)
);

-- 5. Bootstrap default accounts for every existing user (personal)
INSERT OR IGNORE INTO accounts (id, user_id, name, type, currency, created_at, updated_at)
SELECT
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
  substr(lower(hex(randomblob(2))),2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) ||
  substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
  id, '个人账户', 'personal', 'CNY',
  strftime('%Y-%m-%dT%H:%M:%fZ','now'),
  strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM users
WHERE NOT EXISTS (
  SELECT 1 FROM accounts WHERE accounts.user_id = users.id AND accounts.type = 'personal'
);

-- 5b. Bootstrap default accounts (public/company)
INSERT OR IGNORE INTO accounts (id, user_id, name, type, currency, created_at, updated_at)
SELECT
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
  substr(lower(hex(randomblob(2))),2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) ||
  substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))),
  id, '公司账户', 'public', 'CNY',
  strftime('%Y-%m-%dT%H:%M:%fZ','now'),
  strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM users
WHERE NOT EXISTS (
  SELECT 1 FROM accounts WHERE accounts.user_id = users.id AND accounts.type = 'public'
);

-- 6. New transactions schema
CREATE TABLE IF NOT EXISTS transactions_v9 (
  id                TEXT    NOT NULL PRIMARY KEY,
  user_id           TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id          TEXT    NOT NULL,
  direction         TEXT    NOT NULL CHECK(direction IN ('debit','credit')),
  account_id        TEXT    NOT NULL REFERENCES accounts(id),
  amount_cents      INTEGER NOT NULL CHECK(amount_cents > 0),
  currency          TEXT    NOT NULL DEFAULT 'CNY',
  exchange_rate     REAL    NOT NULL DEFAULT 1.0,
  base_amount_cents INTEGER NOT NULL,
  type              TEXT    NOT NULL CHECK(type IN ('income','expense','transfer')),
  category_id       TEXT    REFERENCES categories(id) ON DELETE SET NULL,
  category          TEXT    NOT NULL DEFAULT '',
  reimb_status      TEXT    NOT NULL DEFAULT 'none'
                    CHECK(reimb_status IN ('none','pending','reimbursed')),
  reimb_to_account  TEXT    REFERENCES accounts(id) ON DELETE SET NULL,
  reimbursement_id  TEXT    REFERENCES reimbursements(id),
  project_id        TEXT    REFERENCES projects(id) ON DELETE SET NULL,
  project           TEXT,
  note              TEXT    NOT NULL DEFAULT '',
  attachment_key    TEXT,
  uploaded          INTEGER NOT NULL DEFAULT 0 CHECK(uploaded IN (0,1)),
  idempotency_key   TEXT    UNIQUE,
  txn_date          TEXT    NOT NULL,
  created_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- 7. Migrate existing transactions data
INSERT OR IGNORE INTO transactions_v9 (
  id, user_id, group_id, direction, account_id,
  amount_cents, currency, exchange_rate, base_amount_cents,
  type, category_id, category,
  reimb_status, reimb_to_account, reimbursement_id,
  project_id, project,
  note, uploaded,
  txn_date, created_at, updated_at
)
SELECT
  t.id,
  t.user_id,
  t.id,
  CASE t.direction WHEN 'income' THEN 'credit' ELSE 'debit' END,
  CASE t.source WHEN 'personal' THEN a_p.id ELSE a_c.id END,
  CAST(ROUND(t.amount_yuan * 100) AS INTEGER),
  t.currency,
  1.0,
  CAST(ROUND(t.amount_yuan * 100) AS INTEGER),
  t.direction,
  NULL,
  t.category,
  CASE
    WHEN t.source = 'personal' AND t.direction = 'expense' AND t.reimbursed = 0 THEN 'pending'
    WHEN t.source = 'personal' AND t.direction = 'expense' AND t.reimbursed = 1 THEN 'reimbursed'
    ELSE 'none'
  END,
  CASE WHEN t.source = 'personal' AND t.direction = 'expense' THEN a_p.id ELSE NULL END,
  t.reimbursement_id,
  t.project_id,
  (SELECT p.name FROM projects p WHERE p.id = t.project_id),
  t.note,
  t.uploaded,
  strftime('%Y-%m-%d', datetime(t.occurred_at, 'unixepoch')),
  strftime('%Y-%m-%dT%H:%M:%fZ', datetime(t.created_at, 'unixepoch')),
  strftime('%Y-%m-%dT%H:%M:%fZ', datetime(t.updated_at, 'unixepoch'))
FROM transactions t
LEFT JOIN accounts a_p ON a_p.user_id = t.user_id AND a_p.type = 'personal'
LEFT JOIN accounts a_c ON a_c.user_id = t.user_id AND a_c.type = 'public';

-- 8. Swap tables
DROP TABLE IF EXISTS transactions;

ALTER TABLE transactions_v9 RENAME TO transactions;

-- 9. Seed account balance_cents from migrated transactions
UPDATE accounts SET
  balance_cents = (
    SELECT COALESCE(SUM(
      CASE direction WHEN 'credit' THEN base_amount_cents ELSE -base_amount_cents END
    ), 0)
    FROM transactions
    WHERE transactions.account_id = accounts.id
  ),
  version    = version + 1,
  updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now');

-- 10. Indexes
CREATE INDEX IF NOT EXISTS idx_txn_user_date     ON transactions(user_id, txn_date DESC);
CREATE INDEX IF NOT EXISTS idx_txn_account       ON transactions(account_id);
CREATE INDEX IF NOT EXISTS idx_txn_group         ON transactions(group_id) WHERE type = 'transfer';
CREATE INDEX IF NOT EXISTS idx_txn_pending_reimb ON transactions(user_id, reimb_to_account, base_amount_cents) WHERE reimb_status = 'pending';
CREATE INDEX IF NOT EXISTS idx_txn_user_project  ON transactions(user_id, project_id) WHERE project_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_accounts_user     ON accounts(user_id) WHERE is_active = 1;
CREATE INDEX IF NOT EXISTS idx_audit_row         ON audit_log(table_name, row_id);
CREATE INDEX IF NOT EXISTS idx_audit_user_time   ON audit_log(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_categories_user   ON categories(user_id, type);

PRAGMA foreign_keys = ON;
