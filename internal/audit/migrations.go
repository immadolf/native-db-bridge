package audit

const migrationSQL = `
CREATE TABLE IF NOT EXISTS confirmations (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  datasource TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  payload_hash TEXT NOT NULL,
  summary TEXT NOT NULL,
  risk_level TEXT NOT NULL,
  impact_json TEXT NOT NULL,
  status TEXT NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  executed_at TIMESTAMP NULL,
  error_summary TEXT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  datasource TEXT NOT NULL,
  operation_id TEXT NULL,
  confirmation_id TEXT NULL,
  summary TEXT NOT NULL,
  status TEXT NOT NULL,
  elapsed_ms INTEGER NOT NULL DEFAULT 0,
  row_count INTEGER NOT NULL DEFAULT 0,
  error_code TEXT NULL,
  created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS operations (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  datasource TEXT NOT NULL,
  status TEXT NOT NULL,
  confirmation_id TEXT NULL,
  started_at TIMESTAMP NOT NULL,
  finished_at TIMESTAMP NULL,
  cancel_requested_at TIMESTAMP NULL,
  error_code TEXT NULL,
  error_summary TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_confirmations_created_at ON confirmations (created_at);
CREATE INDEX IF NOT EXISTS idx_confirmations_datasource ON confirmations (datasource);
CREATE INDEX IF NOT EXISTS idx_confirmations_status ON confirmations (status);

CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events (created_at);
CREATE INDEX IF NOT EXISTS idx_audit_events_datasource ON audit_events (datasource);
CREATE INDEX IF NOT EXISTS idx_audit_events_confirmation_id ON audit_events (confirmation_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_status ON audit_events (status);

CREATE INDEX IF NOT EXISTS idx_operations_datasource ON operations (datasource);
CREATE INDEX IF NOT EXISTS idx_operations_confirmation_id ON operations (confirmation_id);
CREATE INDEX IF NOT EXISTS idx_operations_status ON operations (status);
`
