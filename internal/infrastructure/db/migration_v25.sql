-- V25: category backup/restore and API inserts persist creation timestamps.
ALTER TABLE categories
ADD COLUMN created_at TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z';

UPDATE categories
SET created_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE created_at = '1970-01-01T00:00:00Z';
