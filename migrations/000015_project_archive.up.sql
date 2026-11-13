CREATE TABLE wb_project_archives (
 project_id text PRIMARY KEY REFERENCES wb_projects(id) ON DELETE CASCADE,
 activity jsonb NOT NULL, imported_at timestamptz NOT NULL DEFAULT now()
);
