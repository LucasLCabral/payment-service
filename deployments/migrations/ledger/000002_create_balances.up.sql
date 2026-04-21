CREATE TABLE balances (
  account_id   UUID        NOT NULL,
  currency     VARCHAR(3)  NOT NULL DEFAULT 'BRL',
  amount_cents BIGINT      NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (account_id, currency)
);
