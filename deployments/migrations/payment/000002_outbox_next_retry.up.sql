-- Adds per-message retry scheduling to the outbox table.
--
-- next_retry_at allows each message to have its own exponential backoff
-- independently of other messages. Without this, a single failing message
-- is retried on every poll cycle, causing unnecessary load.

ALTER TABLE outbox
  ADD COLUMN next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- The new partial index covers the WHERE clause the publisher uses:
--   WHERE status = 'PENDING' AND next_retry_at <= NOW()
-- Ordering by next_retry_at ensures oldest-scheduled messages are processed first.
DROP INDEX IF EXISTS idx_outbox_status_created;

CREATE INDEX idx_outbox_pending
  ON outbox (next_retry_at)
  WHERE status = 'PENDING';
