CREATE TABLE wb_sessions(id_hash text PRIMARY KEY,csrf_token text NOT NULL,expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE wb_runs(
 id text PRIMARY KEY,project_id text NOT NULL REFERENCES wb_projects(id) ON DELETE CASCADE,chapter_id text NOT NULL REFERENCES wb_chapters(id) ON DELETE CASCADE,
 type text NOT NULL,status text NOT NULL,input jsonb NOT NULL,output jsonb NOT NULL DEFAULT '{}',intent_id text NOT NULL UNIQUE,input_hash text NOT NULL,
 epoch bigint NOT NULL,revision_id text NOT NULL DEFAULT '',lease_generation bigint NOT NULL DEFAULT 0,lease_owner text NOT NULL DEFAULT '',lease_until timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),attempt_count integer NOT NULL DEFAULT 0,cancel_requested boolean NOT NULL DEFAULT false,pause_requested boolean NOT NULL DEFAULT false,
 error text NOT NULL DEFAULT '',stale boolean NOT NULL DEFAULT false,usage jsonb NOT NULL DEFAULT '{}',cost numeric,cost_status text NOT NULL DEFAULT 'Unknown',
 created_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz
);
CREATE INDEX wb_runs_pending ON wb_runs(status,next_attempt_at);
CREATE INDEX wb_runs_project ON wb_runs(project_id,created_at DESC);
CREATE TABLE wb_run_attempts(id text PRIMARY KEY,run_id text NOT NULL REFERENCES wb_runs(id) ON DELETE CASCADE,ordinal integer NOT NULL,status text NOT NULL,disposition text NOT NULL DEFAULT '',raw bytea,usage jsonb,request_snapshot jsonb NOT NULL,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,UNIQUE(run_id,ordinal));
CREATE TABLE wb_run_events(id bigserial PRIMARY KEY,run_id text NOT NULL REFERENCES wb_runs(id) ON DELETE CASCADE,kind text NOT NULL,data jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX wb_events_run ON wb_run_events(run_id,id);
