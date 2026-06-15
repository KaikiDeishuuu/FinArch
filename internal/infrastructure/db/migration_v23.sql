-- V23: monthly budgets
CREATE TABLE IF NOT EXISTS budgets (
  id                TEXT    NOT NULL PRIMARY KEY,
  user_id           TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mode              TEXT    NOT NULL CHECK(mode IN ('work','life')),
  period_month      TEXT    NOT NULL CHECK(length(period_month) = 7),
  category          TEXT    NOT NULL DEFAULT '',
  amount_cents      INTEGER NOT NULL CHECK(amount_cents > 0),
  currency          TEXT    NOT NULL DEFAULT 'CNY',
  base_currency     TEXT    NOT NULL DEFAULT 'CNY',
  base_amount_cents INTEGER NOT NULL CHECK(base_amount_cents > 0),
  is_active         INTEGER NOT NULL DEFAULT 1 CHECK(is_active IN (0,1)),
  created_at        TEXT    NOT NULL,
  updated_at        TEXT    NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_budget_active_scope
ON budgets(user_id, mode, period_month, category)
WHERE is_active = 1;

CREATE INDEX IF NOT EXISTS idx_budget_user_mode_month
ON budgets(user_id, mode, period_month)
WHERE is_active = 1;

CREATE INDEX IF NOT EXISTS idx_txn_budget_actuals
ON transactions(user_id, mode, type, txn_date, category);
