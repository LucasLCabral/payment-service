package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LucasLCabral/payment-service/internal/payment/settlement/mocks"
	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestSettlementHandler_IdempotentBehavior(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := mocks.NewMockTransactionRunner(ctrl)
	mockRepo := mocks.NewMockPayment(ctrl)
	mockAudit := mocks.NewMockAudit(ctrl)
	mockLogger := &MockLogger{}

	handler := &Handler{
		tx:    mockTx,
		repo:  mockRepo,
		audit: mockAudit,
		log:   &LoggerWrapper{mockLogger},
	}

	ctx := context.Background()
	paymentID := uuid.New()

	event := pkgledger.SettlementResult{
		PaymentID: paymentID,
		Status:    "SETTLED",
	}

	body, err := json.Marshal(event)
	assert.NoError(t, err)

	msg := amqp.Delivery{
		Body: body,
		Headers: amqp.Table{
			"traceparent": "00-1234567890abcdef-12345678-01",
		},
	}

	// Configura o mock para executar a função de transação e retornar sql.ErrNoRows
	mockTx.EXPECT().
		WithinTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil) // Executa a função passada
		})

	mockRepo.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), paymentID, payment.StatusSettled, "").
		Return(sql.ErrNoRows)

	err = handler.HandleMessage(ctx, msg)

	assert.NoError(t, err, "Handler deve ser idempotente - sql.ErrNoRows deve ser tratado como sucesso")
}

func TestSettlementHandler_DatabaseError_ShouldFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := mocks.NewMockTransactionRunner(ctrl)
	mockRepo := mocks.NewMockPayment(ctrl)
	mockAudit := mocks.NewMockAudit(ctrl)
	mockLogger := &MockLogger{}

	handler := &Handler{
		tx:    mockTx,
		repo:  mockRepo,
		audit: mockAudit,
		log:   &LoggerWrapper{mockLogger},
	}

	ctx := context.Background()
	paymentID := uuid.New()

	event := pkgledger.SettlementResult{
		PaymentID: paymentID,
		Status:    "SETTLED",
	}

	body, err := json.Marshal(event)
	assert.NoError(t, err)

	msg := amqp.Delivery{
		Body: body,
		Headers: amqp.Table{
			"traceparent": "00-1234567890abcdef-12345678-01",
		},
	}

	dbError := errors.New("connection timeout")

	// Configura o mock para executar a função de transação e retornar erro
	mockTx.EXPECT().
		WithinTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil) // Executa a função passada que deve retornar erro
		})

	mockRepo.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), paymentID, payment.StatusSettled, "").
		Return(dbError)

	err = handler.HandleMessage(ctx, msg)

	assert.Error(t, err)
}

func TestSettlementHandler_NormalFlow_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTx := mocks.NewMockTransactionRunner(ctrl)
	mockRepo := mocks.NewMockPayment(ctrl)
	mockAudit := mocks.NewMockAudit(ctrl)
	mockNotifier := mocks.NewMockPaymentStatusNotifier(ctrl)
	mockLogger := &MockLogger{}

	handler := &Handler{
		tx:       mockTx,
		repo:     mockRepo,
		audit:    mockAudit,
		log:      &LoggerWrapper{mockLogger},
		notifier: mockNotifier,
	}

	ctx := context.Background()
	paymentID := uuid.New()

	event := pkgledger.SettlementResult{
		PaymentID: paymentID,
		Status:    "SETTLED",
	}

	body, err := json.Marshal(event)
	assert.NoError(t, err)

	msg := amqp.Delivery{
		Body: body,
		Headers: amqp.Table{
			"traceparent": "00-1234567890abcdef-12345678-01",
		},
	}

	// Configura o mock para executar a função de transação com sucesso
	mockTx.EXPECT().
		WithinTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil) // Executa a função passada
		})
	
	mockRepo.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), paymentID, payment.StatusSettled, "").
		Return(nil)
	
	mockAudit.EXPECT().
		Insert(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	mockNotifier.EXPECT().
		NotifyPaymentStatus(gomock.Any(), paymentID, payment.StatusSettled, "").
		Return(nil)

	err = handler.HandleMessage(ctx, msg)

	assert.NoError(t, err)
}

// MockLogger - mantido temporariamente para simplicidade dos testes
// Em um projeto real, seria melhor criar um mock específico para o logger também
type MockLogger struct{}

func (m *MockLogger) Debug(ctx context.Context, msg string, keysAndValues ...interface{}) {}
func (m *MockLogger) Info(ctx context.Context, msg string, keysAndValues ...interface{})  {}
func (m *MockLogger) Error(ctx context.Context, msg string, keysAndValues ...interface{}) {}
func (m *MockLogger) Warn(ctx context.Context, msg string, keysAndValues ...interface{})  {}
func (m *MockLogger) With(args ...any) MockLogger                                         { return *m }

type LoggerWrapper struct {
	mock *MockLogger
}

func (w *LoggerWrapper) Debug(ctx context.Context, msg string, args ...any) {
	w.mock.Debug(ctx, msg, args...)
}

func (w *LoggerWrapper) Info(ctx context.Context, msg string, args ...any) {
	w.mock.Info(ctx, msg, args...)
}

func (w *LoggerWrapper) Error(ctx context.Context, msg string, args ...any) {
	w.mock.Error(ctx, msg, args...)
}

func (w *LoggerWrapper) Warn(ctx context.Context, msg string, args ...any) {
	w.mock.Warn(ctx, msg, args...)
}

func (w *LoggerWrapper) With(args ...any) logger.Logger {
	return w
}
