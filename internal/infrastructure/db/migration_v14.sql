-- V14: professional nickname uniqueness hardening
-- normalize duplicate nicknames before creating unique index
UPDATE users
SET nickname = nickname || '-' || substr(id, 1, 4)
WHERE id IN (
  SELECT u1.id
  FROM users u1
  JOIN users u2
    ON lower(COALESCE(u1.nickname,'')) = lower(COALESCE(u2.nickname,''))
   AND u1.id > u2.id
  WHERE COALESCE(u1.nickname,'') <> ''
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_nickname_unique
ON users(nickname COLLATE NOCASE)
WHERE deleted_at IS NULL AND nickname <> '';
