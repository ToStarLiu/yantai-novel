CREATE TABLE wb_provider_secrets (
 id text PRIMARY KEY, endpoint text NOT NULL, ciphertext bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE wb_provider_configs ADD COLUMN no_auth boolean NOT NULL DEFAULT false;
