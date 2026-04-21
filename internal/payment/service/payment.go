package service

import (
	"context"
	"database/sql"

	"github.com/LucasLCabral/payment-service/internal/payment/repository"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/pkg/trace"
)

type TransactionRunner interface {
	WithinTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error
}

type Payment struct {
	TX   TransactionRunner
	Repo repository.Payment
}

func NewPayment(tx TransactionRunner, repo repository.Payment) *Payment {
	return &Payment{TX: tx, Repo: repo}
}

func (s *Payment) CreatePayment(ctx context.Context, in *payment.CreatePaymentRequest) (*payment.Payment, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	tid := trace.GetTraceUUID(ctx)

	var result *payment.Payment
	err := s.TX.WithinTransaction(ctx, func(tx *sql.Tx) error {
		existing, err := s.Repo.GetByIdempotencyKeyForUpdate(ctx, tx, in.IdempotencyKey)
		switch {
		case err == nil:
			result = existing
			return nil
		case err == sql.ErrNoRows:
		default:
			return err
		}
		created, err := s.Repo.InsertPaymentWithOutbox(ctx, tx, in, tid)
		if err != nil {
			return err
		}
		result = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Payment) GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.Payment, error) {
	return s.Repo.GetByID(ctx, req.PaymentID)
}
