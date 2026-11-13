CREATE TABLE wb_states (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 chapter_id text NOT NULL REFERENCES wb_chapters(id), chapter_order integer NOT NULL,
 source_revision_id text NOT NULL REFERENCES wb_revisions(id), base_state_id text NOT NULL DEFAULT '',
 snapshot jsonb NOT NULL, valid boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_states_active ON wb_states(project_id,chapter_order) WHERE valid;
CREATE UNIQUE INDEX wb_state_one_source ON wb_states(project_id,source_revision_id) WHERE valid;
CREATE TABLE wb_facts (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 chapter_id text NOT NULL REFERENCES wb_chapters(id), chapter_order integer NOT NULL,
 source_revision_id text NOT NULL REFERENCES wb_revisions(id), state_id text NOT NULL REFERENCES wb_states(id),
 kind text NOT NULL CHECK(kind IN ('Location','Injury','Ownership','Knowledge','Foreshadow')),
 subject text NOT NULL, predicate text NOT NULL, object text NOT NULL, character_id text NOT NULL DEFAULT '',
 truth_status text NOT NULL CHECK(truth_status IN ('True','False','Unknown')), story_time integer,
 evidence jsonb NOT NULL, active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_facts_active ON wb_facts(project_id,chapter_order,character_id) WHERE active;
CREATE TABLE wb_proposals (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 chapter_id text NOT NULL REFERENCES wb_chapters(id), source_revision_id text NOT NULL REFERENCES wb_revisions(id),
 base_state_id text NOT NULL DEFAULT '', epoch bigint NOT NULL, changes jsonb NOT NULL, original_changes jsonb NOT NULL,
 status text NOT NULL CHECK(status IN ('Pending','Applied')), applied_state_id text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_proposals_project ON wb_proposals(project_id,created_at);
CREATE TABLE wb_chunks (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 chapter_id text NOT NULL REFERENCES wb_chapters(id), chapter_order integer NOT NULL,
 revision_id text NOT NULL REFERENCES wb_revisions(id), source_hash text NOT NULL, chunk_order integer NOT NULL,
 content text NOT NULL, tokenizer_version text NOT NULL DEFAULT 'zh-bigram-v1', active boolean NOT NULL DEFAULT true,
 UNIQUE(revision_id,chunk_order)
);
CREATE INDEX wb_chunks_active ON wb_chunks(project_id,chapter_order) WHERE active;
