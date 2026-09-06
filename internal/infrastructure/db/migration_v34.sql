-- V34: backfill settlement for public-account expenses that predate V33.
--
-- V33 added `settled` with a default of 0, so every public-account expense
-- recorded before the feature existed showed up as pending settlement. Those
-- were cleared with finance under whatever process the user had before, so
-- surfacing them as a backlog is noise rather than a finding.
--
-- Scope is deliberately narrow.
--
-- `created_at < applied_at(V33)` limits this to rows that predate the feature.
-- On a database where V33 and V34 run in the same startup that covers every
-- existing row, which is the intent. On one that has already been running V33
-- it leaves alone anything the user has recorded since and chosen to leave
-- unsettled. strftime returns NULL for an unparseable timestamp and NULL < x
-- is NULL, so a malformed row is skipped rather than settled.
--
-- expense + work mode + public account is exactly what ToggleSettled accepts,
-- so every row this marks can still be un-settled from the UI. Marking a row
-- the toggle would reject would strand it in a state nobody can undo.
--
-- settled_at stays NULL on purpose. These were never settled through the app,
-- so there is no real timestamp to record, and stamping the migration's own
-- clock would put a date on a financial record for something that never
-- happened.
--
-- NOTE: the migration runner splits scripts on the statement separator without
-- skipping comments, so no comment in this file may contain that character.
UPDATE transactions
SET settled = 1
WHERE settled = 0
  AND type = 'expense'
  AND mode = 'work'
  AND EXISTS (
    SELECT 1 FROM accounts a
    WHERE a.id = transactions.account_id
      AND a.user_id = transactions.user_id
      AND a.type = 'public'
  )
  AND CAST(strftime('%s', created_at) AS INTEGER) < (
    SELECT applied_at FROM schema_migrations WHERE version = 33
  );
