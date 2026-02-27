PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  code TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS reimbursements (
  id TEXT PRIMARY KEY,
  request_no TEXT NOT NULL UNIQUE,
  applicant TEXT NOT NULL,
  total_yuan REAL NOT NULL CHECK(total_yuan > 0),
  status TEXT NOT NULL CHECK(status IN ('draft','submitted','paid','cancelled')),
  paid_at INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS transactions (
  id TEXT PRIMARY KEY,
  occurred_at INTEGER NOT NULL,
  direction TEXT NOT NULL CHECK(direction IN ('income','expense')),
  source TEXT NOT NULL CHECK(source IN ('company','personal')),
  category TEXT NOT NULL,
  amount_yuan REAL NOT NULL CHECK(amount_yuan > 0),
  currency TEXT NOT NULL DEFAULT 'CNY',
  note TEXT NOT NULL DEFAULT '',
  project_id TEXT,
  reimbursed INTEGER NOT NULL DEFAULT 0 CHECK(reimbursed IN (0,1)),
  reimbursement_id TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  FOREIGN KEY(project_id) REFERENCES projects(id),
  FOREIGN KEY(reimbursement_id) REFERENCES reimbursements(id)
);

CREATE TABLE IF NOT EXISTS reimbursement_items (
  reimbursement_id TEXT NOT NULL,
  transaction_id TEXT NOT NULL,
  amount_yuan REAL NOT NULL CHECK(amount_yuan > 0),
  PRIMARY KEY (reimbursement_id, transaction_id),
  UNIQUE (transaction_id),
  FOREIGN KEY(reimbursement_id) REFERENCES reimbursements(id) ON DELETE CASCADE,
  FOREIGN KEY(transaction_id) REFERENCES transactions(id)
);

CREATE INDEX IF NOT EXISTS idx_tx_source_reimbursed
ON transactions(source, reimbursed, direction);

CREATE INDEX IF NOT EXISTS idx_tx_project_time
ON transactions(project_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_tx_category_time
ON transactions(category, occurred_at);

CREATE INDEX IF NOT EXISTS idx_reim_status_created
ON reimbursements(status, created_at);

CREATE INDEX IF NOT EXISTS idx_reim_items_reim
ON reimbursement_items(reimbursement_id);
