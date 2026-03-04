-- V15: allow duplicate nicknames (display-only field)
DROP INDEX IF EXISTS idx_users_nickname_unique;
