-- Ledger DB: double-entry bookkeeping
CREATE TABLE ledger_entries (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id       UUID        NOT NULL,
  idempotency_key  UUID        NOT NULL UNIQUE,
  account_id       UUID        NOT NULL,
  amount_cents     BIGINT      NOT NULL CHECK (amount_cents > 0),
  currency         VARCHAR(3)  NOT NULL DEFAULT 'BRL',
  direction        VARCHAR(6)  NOT NULL CHECK (direction IN ('CREDIT','DEBIT')),
  status           VARCHAR(20) NOT NULL DEFAULT 'SETTLED'
                   CHECK (status IN ('SETTLED','DECLINED')),
  decline_reason   VARCHAR(255),
  trace_id         UUID        NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ledger_payment_id ON ledger_entries(payment_id);
CREATE INDEX idx_ledger_account    ON ledger_entries(account_id);
