ALTER TABLE wb_projects ADD COLUMN writing_skills_version bigint NOT NULL DEFAULT 0;
CREATE TABLE wb_writing_skills (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 name text NOT NULL, version bigint NOT NULL DEFAULT 1, enabled boolean NOT NULL DEFAULT false,
 priority integer NOT NULL DEFAULT 0, task_types jsonb NOT NULL DEFAULT '[]', payload jsonb NOT NULL,
 hash text NOT NULL, deleted boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX wb_writing_skill_name ON wb_writing_skills(project_id,lower(name)) WHERE NOT deleted;
CREATE TABLE wb_writing_skill_versions (
 skill_id text NOT NULL REFERENCES wb_writing_skills(id) ON DELETE CASCADE,
 project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 version bigint NOT NULL, snapshot jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(skill_id,version)
);
