-- Adds a PROCESSING status to the outbox table to handle crash recovery.
--
-- Problem being solved:
--   With only PENDING/PUBLISHED/FAILED, if a publisher pod crashes after
--   picking a batch (FOR UPDATE SKIP LOCKED releases the lock on rollback)
--   the rows return to PENDING and are retried — safe. However, if the pod
--   crashes after tx.Commit() but the RabbitMQ publish had already succeeded,
--   those rows are stuck as PROCESSING forever without this recovery mechanism.
--
-- How it works:
--   1. Publisher marks rows PROCESSING before publishing (separate tx).
--   2. After publishing, marks PUBLISHED or schedules retry.
--   3. A watchdog job resets rows stuck in PROCESSING for > 5 minutes back
--      to PENDING so another publisher can pick them up.

ALTER TABLE outbox
  ALTER COLUMN status TYPE VARCHAR(20),
  ALTER COLUMN status DROP DEFAULT;

ALTER TABLE outbox
  DROP CONSTRAINT IF EXISTS outbox_status_check;

ALTER TABLE outbox
  ADD CONSTRAINT outbox_status_check
    CHECK (status IN ('PENDING','PROCESSING','PUBLISHED','FAILED'));

ALTER TABLE outbox
  ALTER COLUMN status SET DEFAULT 'PENDING';

ALTER TABLE outbox
  ADD COLUMN processing_since TIMESTAMPTZ;

-- Index for the watchdog to find stuck messages efficiently.
CREATE INDEX idx_outbox_stuck_processing
  ON outbox (processing_since)
  WHERE status = 'PROCESSING';
