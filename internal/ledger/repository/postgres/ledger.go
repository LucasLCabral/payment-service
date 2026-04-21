package postgres

import (
	"context"
	"database/sql"

	"github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/google/uuid"
)

type Repository struct {
	DB *sql.DB
}

// GetBalance trava a linha com FOR UPDATE. Retorna sql.ErrNoRows se a conta não existe.
func (r *Repository) GetBalance(ctx context.Context, tx *sql.Tx, accountID uuid.UUID, currency string) (int64, error) {
	var amount int64
	err := tx.QueryRowContext(ctx,
		`SELECT amount_cents FROM balances WHERE account_id = $1 AND currency = $2 FOR UPDATE`,
		accountID, currency,
	).Scan(&amount)
	return amount, err
}

// DebitCredit debita do payer e credita no payee.
func (r *Repository) DebitCredit(ctx context.Context, tx *sql.Tx, payerID, payeeID uuid.UUID, currency string, amount int64) error {
	const debit = `UPDATE balances SET amount_cents = amount_cents - $1, updated_at = NOW() WHERE account_id = $2 AND currency = $3`
	if _, err := tx.ExecContext(ctx, debit, amount, payerID, currency); err != nil {
		return err
	}

	const credit = `
		INSERT INTO balances (account_id, currency, amount_cents, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (account_id, currency)
		DO UPDATE SET amount_cents = balances.amount_cents + $3, updated_at = NOW()`
	_, err := tx.ExecContext(ctx, credit, payeeID, currency, amount)
	return err
}

// InsertEntries grava DEBIT + CREDIT. Retorna false se idempotency_key já existe.
func (r *Repository) InsertEntries(ctx context.Context, tx *sql.Tx, debit, credit *ledger.Entry) (bool, error) {
	inserted, err := insertOne(ctx, tx, debit)
	if err != nil || !inserted {
		return false, err
	}

	if _, err := insertOne(ctx, tx, credit); err != nil {
		return false, err
	}

	return true, nil
}

func insertOne(ctx context.Context, tx *sql.Tx, e *ledger.Entry) (bool, error) {
	const q = `
		INSERT INTO ledger_entries (
			payment_id, idempotency_key, amount_cents, currency,
			direction, payer_id, payee_id, status, decline_reason, trace_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (idempotency_key) DO NOTHING`

	var declineArg any
	if e.DeclineReason != "" {
		declineArg = e.DeclineReason
	}

	res, err := tx.ExecContext(ctx, q,
		e.PaymentID, e.IdempotencyKey, e.AmountCents, e.Currency,
		string(e.Direction), e.PayerID, e.PayeeID, string(e.Status), declineArg, e.TraceID,
	)
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
