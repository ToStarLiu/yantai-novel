-- name: EnsureProject :one
INSERT INTO projects(id,project_key) VALUES($1,$2)
ON CONFLICT(project_key) DO UPDATE SET project_key=EXCLUDED.project_key RETURNING id;

-- name: InsertJob :execrows
INSERT INTO generation_jobs(id,project_id,intent_id,input_hash,input_snapshot,status)
VALUES($1,$2,$3,$4,$5,'Pending') ON CONFLICT(intent_id) DO NOTHING;

-- name: GetJobByIntent :one
SELECT * FROM generation_jobs WHERE intent_id=$1;

-- name: GetJob :one
SELECT * FROM generation_jobs WHERE id=$1;

-- name: LockJob :one
SELECT * FROM generation_jobs WHERE id=$1 FOR UPDATE;

-- name: StartJob :exec
UPDATE generation_jobs SET status='Running' WHERE id=$1;

-- name: InsertAttempt :exec
INSERT INTO job_attempts(id,job_id,step,input_hash,prompt_version,provider,model,request_snapshot,price_snapshot,outcome)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'Running');

-- name: InsertArtifact :exec
INSERT INTO run_artifacts(id,job_id,kind,content) VALUES($1,$2,$3,$4);

-- name: FinishAttempt :execrows
UPDATE job_attempts SET outcome=$2,raw_artifact_id=$3,provider_request_id=$4,usage=$5,error=$6,finished_at=now()
WHERE id=$1 AND outcome='Running' AND job_id=$7;

-- name: CompleteJob :execrows
UPDATE generation_jobs SET status=$2,finished_at=now() WHERE id=$1 AND status IN ('Pending','Running');

-- name: ListArtifacts :many
SELECT * FROM run_artifacts WHERE job_id=$1 ORDER BY created_at,id;

-- name: ListAttempts :many
SELECT * FROM job_attempts WHERE job_id=$1 ORDER BY started_at,id;
