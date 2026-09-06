-- V33: public-account settlement flag.
--
-- Reimbursement (reimb_status) means "the company pays the user back for money
-- they fronted", so it is restricted to expenses booked against a personal
-- account. Expenses paid straight out of a public account have nothing to
-- reimburse, but they still need to be cleared with finance. That lifecycle
-- gets its own flag so it never feeds the reimbursement math in WORK-mode
-- statistics, which adds reimbursed amounts back into the user's net.
ALTER TABLE transactions ADD COLUMN settled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE transactions ADD COLUMN settled_at INTEGER;
