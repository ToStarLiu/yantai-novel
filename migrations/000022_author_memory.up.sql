CREATE TABLE wb_author_memory_settings (
 owner_id text PRIMARY KEY, version bigint NOT NULL DEFAULT 0,
 context_version bigint NOT NULL DEFAULT 0,
 use_memory boolean NOT NULL DEFAULT true, suggest_memories boolean NOT NULL DEFAULT true,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wb_author_memories (
 id text PRIMARY KEY, owner_id text NOT NULL,
 scope text NOT NULL CHECK(scope IN ('author','book')),
 project_id text NOT NULL DEFAULT '', kind text NOT NULL,
 title text NOT NULL, content text NOT NULL,
 status text NOT NULL CHECK(status IN ('active','pending','disabled','rejected')),
 pinned boolean NOT NULL DEFAULT false, version bigint NOT NULL DEFAULT 1,
 source_session_id text NOT NULL DEFAULT '', source_turn_id text NOT NULL DEFAULT '', source_quote text NOT NULL DEFAULT '',
 fingerprint text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((scope='author' AND project_id='') OR (scope='book' AND project_id<>''))
);
CREATE UNIQUE INDEX wb_author_memories_dedup ON wb_author_memories(owner_id,scope,project_id,fingerprint);
CREATE INDEX wb_author_memories_owner ON wb_author_memories(owner_id,status,project_id);
-- Events deliberately retain no prose or source quotations after forgetting.
CREATE TABLE wb_author_memory_events (
 id text PRIMARY KEY, owner_id text NOT NULL, memory_id text NOT NULL,
 version bigint NOT NULL, action text NOT NULL, content_hash text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
