-- V26: durable outbox for attachment objects that must be removed from
-- external storage. There is intentionally no users foreign key: queue rows
-- must survive permanent deletion of their owner so cleanup can be retried.
CREATE TABLE IF NOT EXISTS attachment_deletion_queue (
  storage_key TEXT    NOT NULL PRIMARY KEY,
  user_id     TEXT    NOT NULL,
  attempts    INTEGER NOT NULL DEFAULT 0 CHECK(attempts >= 0),
  last_error  TEXT,
  created_at  TEXT    NOT NULL,
  updated_at  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attachment_deletion_queue_pending
ON attachment_deletion_queue(updated_at, created_at, storage_key);
