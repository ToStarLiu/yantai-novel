-- Shared runs retain only ownership/version fences; private model context stays here.
CREATE TABLE wb_run_author_contexts (
 run_id text PRIMARY KEY REFERENCES wb_runs(id) ON DELETE CASCADE,
 owner_id text NOT NULL,
 principal jsonb NOT NULL,
 memory_snapshot jsonb NOT NULL,
 augmented_request jsonb NOT NULL,
 excluded_reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_run_author_contexts_owner ON wb_run_author_contexts(owner_id,run_id);
CREATE TABLE wb_production_author_contexts (
 production_id text PRIMARY KEY REFERENCES wb_productions(id) ON DELETE CASCADE,
 owner_id text NOT NULL,
 principal jsonb NOT NULL,
 memory_baseline jsonb,
 ignore_memory boolean NOT NULL DEFAULT false
);
CREATE TABLE wb_candidate_author_contexts (
 group_id text PRIMARY KEY REFERENCES wb_candidate_groups(id) ON DELETE CASCADE,
 owner_id text NOT NULL,
 principal jsonb NOT NULL,
 memory_baseline jsonb,
 ignore_memory boolean NOT NULL DEFAULT false
);
