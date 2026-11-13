CREATE TABLE wb_research_reports (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 topic text NOT NULL, status text NOT NULL, sources jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_research_project ON wb_research_reports(project_id,created_at);
