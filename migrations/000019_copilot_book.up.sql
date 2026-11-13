ALTER TABLE wb_copilot_sessions ADD COLUMN scope text NOT NULL DEFAULT 'chapter' CHECK(scope IN ('book','chapter'));
CREATE TABLE wb_book_plan_proposals (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 owner_id text NOT NULL, turn_id text NOT NULL REFERENCES wb_copilot_turns(id) ON DELETE CASCADE,
 tool_call_id text NOT NULL, input_hash text NOT NULL, input jsonb NOT NULL,
 title text NOT NULL, text text NOT NULL, expected_version bigint NOT NULL,
 adopted_version bigint, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(turn_id,tool_call_id)
);
CREATE INDEX wb_book_plans_project ON wb_book_plan_proposals(project_id,created_at DESC);
