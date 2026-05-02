DROP INDEX IF EXISTS idx_outbox_stuck_processing;

ALTER TABLE outbox DROP COLUMN IF EXISTS processing_since;

ALTER TABLE outbox DROP CONSTRAINT IF EXISTS outbox_status_check;

ALTER TABLE outbox
  ADD CONSTRAINT outbox_status_check
    CHECK (status IN ('PENDING','PUBLISHED','FAILED'));
