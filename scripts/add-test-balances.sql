-- Script to add balance to test accounts for stress testing
-- Execute in ledger_db (port 5433)

-- Default accounts for k6 tests
INSERT INTO balances (account_id, currency, amount_cents) 
VALUES 
  -- Default payer: R$ 1.000.000,00 (100 mil reais)
  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 'BRL', 100000000),
  
  -- Default payee: R$ 0,00 (will receive the payments)
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 'BRL', 0)

-- Resolve conflicts if the accounts already exist
ON CONFLICT (account_id, currency) 
DO UPDATE SET 
  amount_cents = EXCLUDED.amount_cents,
  updated_at = NOW();

-- Add some extra accounts for realistic test scenarios
INSERT INTO balances (account_id, currency, amount_cents) 
VALUES 
  -- Accounts with varying balances for realistic test scenarios
  ('cccccccc-cccc-cccc-cccc-cccccccccccc', 'BRL', 50000000),  -- R$ 500.000
  ('dddddddd-dddd-dddd-dddd-dddddddddddd', 'BRL', 25000000),  -- R$ 250.000  
  ('eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee', 'BRL', 10000000),  -- R$ 100.000
  ('ffffffff-ffff-ffff-ffff-ffffffffffff', 'BRL', 5000000),   -- R$ 50.000
  ('11111111-1111-1111-1111-111111111111', 'BRL', 1000000),   -- R$ 10.000
  ('22222222-2222-2222-2222-222222222222', 'BRL', 500000)     -- R$ 5.000

ON CONFLICT (account_id, currency) 
DO UPDATE SET 
  amount_cents = EXCLUDED.amount_cents,
  updated_at = NOW();

-- Show current balances
SELECT 
  account_id,
  currency,
  amount_cents,
  ROUND(amount_cents / 100.0, 2) AS amount_reais,
  updated_at
FROM balances 
ORDER BY amount_cents DESC;