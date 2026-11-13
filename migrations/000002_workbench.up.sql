CREATE TABLE wb_projects (
 id text PRIMARY KEY, title text NOT NULL, description text NOT NULL DEFAULT '', genre text NOT NULL DEFAULT '',
 epoch bigint NOT NULL DEFAULT 1, health text NOT NULL DEFAULT 'Ready', state_version_id text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wb_settings (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 kind text NOT NULL, name text NOT NULL, content text NOT NULL DEFAULT ''
);
CREATE INDEX wb_settings_project ON wb_settings(project_id);
CREATE TABLE wb_chapters (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 volume text NOT NULL DEFAULT '', title text NOT NULL, sort_order integer NOT NULL,
 blueprint text NOT NULL DEFAULT '', blueprint_version bigint NOT NULL DEFAULT 1,
 current_revision_id text NOT NULL DEFAULT '', final_revision_id text NOT NULL DEFAULT '',
 needs_review boolean NOT NULL DEFAULT false, deleted boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_chapters_project ON wb_chapters(project_id,sort_order);
CREATE TABLE wb_revisions (
 id text PRIMARY KEY, chapter_id text NOT NULL REFERENCES wb_chapters(id) ON DELETE CASCADE,
 parent_id text NOT NULL DEFAULT '', document jsonb NOT NULL, plain_text text NOT NULL, hash text NOT NULL,
 source text NOT NULL, run_id text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_revisions_chapter ON wb_revisions(chapter_id,created_at);
CREATE FUNCTION wb_revision_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'chapter revisions are immutable'; END $$;
CREATE TRIGGER wb_revision_immutable BEFORE UPDATE ON wb_revisions FOR EACH ROW EXECUTE FUNCTION wb_revision_immutable();
CREATE TABLE wb_impacts (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 earliest_order integer NOT NULL, reason text NOT NULL, chapter_ids jsonb NOT NULL DEFAULT '[]',
 created_at timestamptz NOT NULL DEFAULT now()
);
