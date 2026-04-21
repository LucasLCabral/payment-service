package repository

import (
	"context"
	"database/sql"

	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/google/uuid"
)

type Repository interface {
	// GetBalance trava a linha do saldo para evitar gasto duplo concorrente.
	GetBalance(ctx context.Context, tx *sql.Tx, accountID uuid.UUID, currency string) (int64, error)

	// DebitCredit debita do payer e credita no payee atomicamente.
	DebitCredit(ctx context.Context, tx *sql.Tx, payerID, payeeID uuid.UUID, currency string, amount int64) error

	// InsertEntries grava DEBIT + CREDIT. Retorna false se idempotency_key já existe.
	InsertEntries(ctx context.Context, tx *sql.Tx, debit, credit *pkgledger.Entry) (inserted bool, err error)
}
