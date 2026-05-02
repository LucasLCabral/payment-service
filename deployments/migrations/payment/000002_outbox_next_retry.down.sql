DROP INDEX IF EXISTS idx_outbox_pending;

ALTER TABLE outbox DROP COLUMN IF EXISTS next_retry_at;

CREATE INDEX idx_outbox_status_created
  ON outbox (status, created_at)
  WHERE status = 'PENDING';
