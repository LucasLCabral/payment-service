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

func (s *Payment) CreatePayment(ctx context.Context, req *payment.CreatePaymentRequest) (*payment.Payment, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	tid := trace.GetTraceUUID(ctx)

	var result *payment.Payment
	err := s.TX.WithinTransaction(ctx, func(tx *sql.Tx) error {
		existing, err := s.Repo.FindByIdempotencyKey(ctx, tx, req.IdempotencyKey)
		if err == nil {
			result = existing
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}

		result, err = s.Repo.Create(ctx, tx, req, tid)
		return err
	})
	if err != nil {
		return nil, err
	}
	
	return result, nil
}

func (s *Payment) GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.Payment, error) {
	return s.Repo.FindByID(ctx, req.PaymentID)
}
