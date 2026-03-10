-- V21: repair account balance cache deterministically from normalized base amounts.
-- This is idempotent and repairs historical corruption caused by legacy triggers
-- that summed source amount_cents instead of base_amount_cents.
UPDATE accounts
SET balance_cents = (
  SELECT COALESCE(SUM(
    CASE t.direction WHEN 'credit' THEN t.base_amount_cents ELSE -t.base_amount_cents END
  ), 0)
  FROM transactions t
  WHERE t.account_id = accounts.id
);
