ALTER TABLE users ADD COLUMN IF NOT EXISTS external_subject text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS users_external_subject_uidx ON users(external_subject) WHERE external_subject IS NOT NULL;
