CREATE TABLE wb_studio_documents (
 project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 kind text NOT NULL, id text NOT NULL, version bigint NOT NULL DEFAULT 1,
 data jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(project_id,kind,id)
);
CREATE TABLE wb_studio_history (
 project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 kind text NOT NULL, id text NOT NULL, version bigint NOT NULL,
 data jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(project_id,kind,id,version)
);
CREATE TABLE wb_branch_links (
 id text PRIMARY KEY, source_project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 target_project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 source_epoch bigint NOT NULL, source_title text NOT NULL, title text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(target_project_id)
);
