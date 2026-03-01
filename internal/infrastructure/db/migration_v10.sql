-- V10: nickname (display name, mutable, optional)
ALTER TABLE users ADD COLUMN nickname TEXT NOT NULL DEFAULT '';
-- Backfill: set nickname = username for existing users
UPDATE users SET nickname = username WHERE nickname = '';
