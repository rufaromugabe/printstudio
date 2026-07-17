ALTER TABLE assets ADD COLUMN IF NOT EXISTS declared_sha256 text NOT NULL DEFAULT repeat('0',64);
ALTER TABLE assets ADD CONSTRAINT assets_sha256_format CHECK(declared_sha256 ~ '^[0-9a-f]{64}$') NOT VALID;
CREATE INDEX IF NOT EXISTS assets_pending_created_idx ON assets(created_at) WHERE status='pending';
