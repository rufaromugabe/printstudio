CREATE TABLE IF NOT EXISTS production_proofs(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  design_id uuid NOT NULL REFERENCES designs(id) ON DELETE CASCADE,
  design_version int NOT NULL,
  method text NOT NULL,
  artifact_sha256 text NOT NULL,
  width_mm double precision NOT NULL,
  height_mm double precision NOT NULL,
  checklist jsonb NOT NULL DEFAULT '{}'::jsonb,
  status text NOT NULL CHECK(status IN('pending','approved','rejected')) DEFAULT 'pending',
  created_by uuid NOT NULL REFERENCES users(id),
  approved_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  approved_at timestamptz,
  notes text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS production_proofs_design_idx ON production_proofs(design_id, created_at DESC);
CREATE INDEX IF NOT EXISTS production_proofs_workspace_status_idx ON production_proofs(workspace_id, status, created_at DESC);
