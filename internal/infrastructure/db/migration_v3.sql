-- V3 migration: add uploaded field to transactions
ALTER TABLE transactions ADD COLUMN uploaded INTEGER NOT NULL DEFAULT 0;
