CREATE TABLE IF NOT EXISTS assets(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES users(id),
  file_name text NOT NULL,
  object_key text NOT NULL,
  content_type text NOT NULL CHECK(content_type IN('image/png','image/jpeg')),
  declared_size bigint NOT NULL CHECK(declared_size>0 AND declared_size<=26214400),
  declared_sha256 text NOT NULL DEFAULT repeat('0',64),
  actual_size bigint,
  width integer,
  height integer,
  status text NOT NULL CHECK(status IN('pending','validated','rejected')),
  rejection_reason text,
  validated_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS assets_workspace_created_idx ON assets(workspace_id,created_at DESC);
