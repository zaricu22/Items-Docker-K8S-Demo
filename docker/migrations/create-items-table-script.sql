-- Idempotent (IF NOT EXISTS), so re-running the db-migration service is harmless.
-- Same statement as k8s/base/controllers/migration-job.yaml.
CREATE TABLE IF NOT EXISTS items (
  id serial PRIMARY KEY,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
