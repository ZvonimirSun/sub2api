-- A system probe belongs to an upstream account, not a billable user/API key.
ALTER TABLE usage_logs ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE usage_logs ALTER COLUMN api_key_id DROP NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_logs_system_probe_request
    ON usage_logs (request_id) WHERE inbound_endpoint = '/internal/modeltrace';
