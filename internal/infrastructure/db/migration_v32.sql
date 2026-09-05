-- V32: retire reimbursement-status balance mutations.
--
-- A reimbursement marks an existing personal expense as settled, it is not a
-- posted account transaction. Account balances, balance history, migration
-- repair, and restore repair must therefore all use the same invariant:
-- credits add base_amount_cents and debits subtract it, independent of
-- reimb_status. ApplyTriggers removes the two legacy reimbursement triggers
-- after migrations complete. Rebuild the cache once so upgraded databases do
-- not retain balance deltas previously introduced by those triggers.
UPDATE accounts
SET balance_cents = (
  SELECT COALESCE(SUM(
    CASE t.direction WHEN 'credit' THEN t.base_amount_cents ELSE -t.base_amount_cents END
  ), 0)
  FROM transactions t
  WHERE t.account_id = accounts.id
);
