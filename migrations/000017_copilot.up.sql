CREATE TABLE wb_copilot_sessions (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 owner_id text NOT NULL, title text NOT NULL, chapter_id text NOT NULL DEFAULT '',
 messages jsonb NOT NULL DEFAULT '[]', context_key text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_copilot_sessions_owner ON wb_copilot_sessions(project_id,owner_id,updated_at DESC);
CREATE TABLE wb_copilot_turns (
 id text PRIMARY KEY, session_id text NOT NULL REFERENCES wb_copilot_sessions(id) ON DELETE CASCADE,
 project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE, owner_id text NOT NULL,
 intent_id text NOT NULL UNIQUE, input_hash text NOT NULL, input jsonb NOT NULL,
 status text NOT NULL DEFAULT 'Pending', text text NOT NULL DEFAULT '', error text NOT NULL DEFAULT '',
 artifacts jsonb NOT NULL DEFAULT '[]', stale boolean NOT NULL DEFAULT false,
 cancel_requested boolean NOT NULL DEFAULT false, lease_owner text NOT NULL DEFAULT '', lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz
);
CREATE UNIQUE INDEX wb_copilot_one_active ON wb_copilot_turns(session_id) WHERE status IN ('Pending','Running');
CREATE INDEX wb_copilot_queue ON wb_copilot_turns(status,created_at);
CREATE TABLE wb_copilot_attempts (
 id text PRIMARY KEY, turn_id text NOT NULL REFERENCES wb_copilot_turns(id) ON DELETE CASCADE,
 project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,
 ordinal integer NOT NULL, status text NOT NULL, policy jsonb NOT NULL, request jsonb NOT NULL,
 usage jsonb NOT NULL DEFAULT '{}', reserved_cost double precision, raw jsonb NOT NULL DEFAULT '{}',
 started_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz,
 UNIQUE(turn_id,ordinal)
);
CREATE TABLE wb_copilot_events (
 id bigserial PRIMARY KEY, turn_id text NOT NULL REFERENCES wb_copilot_turns(id) ON DELETE CASCADE,
 kind text NOT NULL, data jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wb_copilot_events_turn ON wb_copilot_events(turn_id,id);
