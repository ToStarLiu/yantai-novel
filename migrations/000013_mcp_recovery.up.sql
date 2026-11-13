-- Earlier development snapshots applied 000012 before intent/server audit tables
-- were added. This additive migration also upgrades those databases safely.
CREATE TABLE IF NOT EXISTS wb_mcp_configs(id text PRIMARY KEY,name text NOT NULL,url text NOT NULL,api_key_env text NOT NULL DEFAULT '',allowed_tools jsonb NOT NULL DEFAULT '[]',allow_loopback boolean NOT NULL DEFAULT false,enabled boolean NOT NULL DEFAULT true,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS wb_mcp_calls(id text PRIMARY KEY,config_id text NOT NULL REFERENCES wb_mcp_configs(id),tool_name text NOT NULL,arguments_hash text NOT NULL,status text NOT NULL,duration_ms bigint NOT NULL,error text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS wb_mcp_run_inputs(intent_id text PRIMARY KEY,request_hash text NOT NULL,agent_id text NOT NULL,status text NOT NULL,results jsonb NOT NULL DEFAULT '[]',error text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz);
CREATE TABLE IF NOT EXISTS wb_mcp_server_calls(id text PRIMARY KEY,tool_name text NOT NULL,arguments_hash text NOT NULL,status text NOT NULL,duration_ms bigint NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS wb_mcp_calls_recent ON wb_mcp_calls(created_at DESC);
CREATE INDEX IF NOT EXISTS wb_mcp_server_calls_recent ON wb_mcp_server_calls(created_at DESC);
