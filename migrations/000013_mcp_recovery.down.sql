-- Preserve 000012 data and business tables. Only indexes introduced here roll back.
DROP INDEX IF EXISTS wb_mcp_calls_recent;
DROP INDEX IF EXISTS wb_mcp_server_calls_recent;
