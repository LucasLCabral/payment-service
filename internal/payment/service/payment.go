package service

import (
	"context"
	"database/sql"

	model "github.com/LucasLCabral/payment-service/internal/payment/models"
	"github.com/LucasLCabral/payment-service/internal/payment/repository"
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

func (s *Payment) CreatePayment(ctx context.Context, req *model.CreatePaymentRequest) (*model.Payment, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	tid := trace.GetTraceUUID(ctx)

	var result *model.Payment
	err := s.TX.WithinTransaction(ctx, func(tx *sql.Tx) error {
		existing, err := s.Repo.GetByIdempotencyKeyForUpdate(ctx, tx, req.IdempotencyKey)
		switch {
		case err == nil:
			result = existing
			return nil
		case err == sql.ErrNoRows:
		default:
			return err
		}
		created, err := s.Repo.InsertPaymentWithOutbox(ctx, tx, req, tid)
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

func (s *Payment) GetPayment(ctx context.Context, req *model.GetPaymentRequest) (*model.Payment, error) {
	return s.Repo.GetByID(ctx, req.PaymentID)
}
