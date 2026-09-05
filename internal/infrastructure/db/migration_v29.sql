-- V29: make integer cents authoritative for reimbursements.
--
-- Legacy REAL values are positive by the original schema. Reject values too
-- close to the int64 boundary before multiplying by 100. The conservative
-- limit leaves enough headroom for binary64 multiplication. Existing values
-- are rounded to the nearest cent with SQLite ROUND (half away from zero for
-- these positive amounts). Any precision already lost in legacy REAL data
-- cannot be recovered, but all writes after this migration preserve cents.
CREATE TABLE _migration_v29_amount_guard (
  valid INTEGER NOT NULL CHECK(valid = 1)
);

INSERT INTO _migration_v29_amount_guard(valid)
SELECT CASE WHEN
  EXISTS (
    SELECT 1 FROM reimbursements
    WHERE total_yuan IS NULL
       OR total_yuan <= 0
       OR total_yuan > 92233720368547700.0
  ) OR EXISTS (
    SELECT 1 FROM reimbursement_items
    WHERE amount_yuan IS NULL
       OR amount_yuan <= 0
       OR amount_yuan > 92233720368547700.0
  )
THEN 0 ELSE 1 END;

ALTER TABLE reimbursements
ADD COLUMN total_cents INTEGER NOT NULL DEFAULT 0 CHECK(total_cents >= 0);

ALTER TABLE reimbursement_items
ADD COLUMN amount_cents INTEGER NOT NULL DEFAULT 0 CHECK(amount_cents >= 0);

UPDATE reimbursements
SET total_cents = CAST(ROUND(total_yuan * 100.0) AS INTEGER);

UPDATE reimbursement_items
SET amount_cents = CAST(ROUND(amount_yuan * 100.0) AS INTEGER);

DROP TABLE _migration_v29_amount_guard;
