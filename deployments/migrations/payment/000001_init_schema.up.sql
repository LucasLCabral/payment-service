-- Payment DB: schema inicial (transactions, outbox, audit_log)

-- Transações (dinheiro em centavos, IDs UUID)
CREATE TABLE transactions (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  idempotency_key  UUID        NOT NULL UNIQUE,
  amount_cents     BIGINT      NOT NULL CHECK (amount_cents > 0),
  currency         VARCHAR(3)  NOT NULL DEFAULT 'BRL',
  status           VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                   CHECK (status IN ('PENDING','SETTLED','DECLINED','FAILED')),
  payer_id         UUID        NOT NULL,
  payee_id         UUID        NOT NULL,
  description      VARCHAR(255),
  decline_reason   VARCHAR(255),
  trace_id         UUID        NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_status   ON transactions(status);
CREATE INDEX idx_transactions_payer_id ON transactions(payer_id);

-- Outbox transacional (publisher faz polling em PENDING)
CREATE TABLE outbox (
  id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  aggregate_id   UUID         NOT NULL,
  aggregate_type VARCHAR(50)  NOT NULL,
  event_type     VARCHAR(100) NOT NULL,
  payload        JSONB        NOT NULL,
  status         VARCHAR(20)  NOT NULL DEFAULT 'PENDING'
                 CHECK (status IN ('PENDING','PUBLISHED','FAILED')),
  attempts       INT          NOT NULL DEFAULT 0,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  published_at   TIMESTAMPTZ
);

CREATE INDEX idx_outbox_status_created
  ON outbox(status, created_at)
  WHERE status = 'PENDING';

-- Auditoria append-only
CREATE TABLE audit_log (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_type  VARCHAR(50) NOT NULL,
  entity_id    UUID        NOT NULL,
  action       VARCHAR(50) NOT NULL,
  old_status   VARCHAR(20),
  new_status   VARCHAR(20),
  trace_id     UUID        NOT NULL,
  actor        VARCHAR(100),
  metadata     JSONB,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_trace  ON audit_log(trace_id);

ALTER TABLE audit_log OWNER TO postgres;
GRANT SELECT, INSERT ON audit_log TO payment_user;
