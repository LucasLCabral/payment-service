package ledger

import (
	"context"
	"database/sql"

	"github.com/LucasLCabral/payment-service/internal/ledger/repository"
	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/google/uuid"
)

type TransactionRunner interface {
	WithinTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error
}

type Service struct {
	tx   TransactionRunner
	repo repository.Repository
	pub  *messaging.Publisher
	log  logger.Logger
}

func NewService(tx TransactionRunner, repo repository.Repository, pub *messaging.Publisher, log logger.Logger) *Service {
	return &Service{tx: tx, repo: repo, pub: pub, log: log}
}

func (s *Service) ProcessPaymentCreated(ctx context.Context, evt *pkgledger.PaymentCreatedEvent, traceID uuid.UUID) error {
	var decision evaluation
	var inserted bool

	err := s.tx.WithinTransaction(ctx, func(tx *sql.Tx) error {
		balance, err := s.repo.GetBalance(ctx, tx, evt.PayerID, evt.Currency)
		if err != nil && err != sql.ErrNoRows {
			return err
		}

		decision = evaluate(evt, balance)

		debitKey := evt.IdempotencyKey
		creditKey := uuid.NewSHA1(debitKey, []byte("credit"))

		debit := &pkgledger.Entry{
			PaymentID:      evt.PaymentID,
			IdempotencyKey: debitKey,
			AccountID:      evt.PayerID,
			AmountCents:    evt.AmountCents,
			Currency:       evt.Currency,
			Direction:      pkgledger.DirectionDebit,
			Status:         decision.status,
			DeclineReason:  decision.reason,
			TraceID:        traceID,
		}

		credit := &pkgledger.Entry{
			PaymentID:      evt.PaymentID,
			IdempotencyKey: creditKey,
			AccountID:      evt.PayeeID,
			AmountCents:    evt.AmountCents,
			Currency:       evt.Currency,
			Direction:      pkgledger.DirectionCredit,
			Status:         decision.status,
			DeclineReason:  decision.reason,
			TraceID:        traceID,
		}

		inserted, err = s.repo.InsertEntries(ctx, tx, debit, credit)
		if err != nil {
			return err
		}

		if inserted && decision.status == pkgledger.EntryStatusSettled {
			return s.repo.DebitCredit(ctx, tx, evt.PayerID, evt.PayeeID, evt.Currency, evt.AmountCents)
		}

		return nil
	})
	if err != nil {
		return err
	}

	if !inserted {
		s.log.Info(ctx, "idempotent skip", "payment_id", evt.PaymentID)
		return nil
	}

	return s.publishSettlement(ctx, evt.PaymentID, decision, traceID)
}
