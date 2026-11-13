CREATE TABLE projects (
    id text PRIMARY KEY,
    project_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE generation_jobs (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id),
    intent_id text NOT NULL UNIQUE,
    input_hash text NOT NULL,
    input_snapshot jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('Pending','Running','Succeeded','Failed','NeedsReconciliation')),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE TABLE run_artifacts (
    id text PRIMARY KEY,
    job_id text NOT NULL REFERENCES generation_jobs(id),
    kind text NOT NULL,
    content bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE job_attempts (
    id text PRIMARY KEY,
    job_id text NOT NULL REFERENCES generation_jobs(id),
    step text NOT NULL,
    input_hash text NOT NULL,
    prompt_version text NOT NULL,
    provider text NOT NULL,
    model text NOT NULL,
    request_snapshot jsonb NOT NULL,
    price_snapshot jsonb NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('Running','Succeeded','Failed','NeedsReconciliation')),
    raw_artifact_id text REFERENCES run_artifacts(id),
    provider_request_id text NOT NULL DEFAULT '',
    usage jsonb,
    error text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    UNIQUE (job_id,step)
);
CREATE INDEX attempts_by_job ON job_attempts(job_id,started_at);
CREATE INDEX artifacts_by_job ON run_artifacts(job_id,created_at);
