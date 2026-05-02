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

func newHandler(ctrl *gomock.Controller) (*Handler, *mocks.MockTransactionRunner, *mocks.MockPayment, *mocks.MockAudit, *mocks.MockPaymentStatusNotifier) {
	mockTx := mocks.NewMockTransactionRunner(ctrl)
	mockRepo := mocks.NewMockPayment(ctrl)
	mockAudit := mocks.NewMockAudit(ctrl)
	mockNotifier := mocks.NewMockPaymentStatusNotifier(ctrl)
	h := &Handler{
		tx:       mockTx,
		repo:     mockRepo,
		audit:    mockAudit,
		log:      &LoggerWrapper{&MockLogger{}},
		notifier: mockNotifier,
	}
	return h, mockTx, mockRepo, mockAudit, mockNotifier
}

func settlementMsg(t *testing.T, paymentID uuid.UUID, status string) amqp.Delivery {
	t.Helper()
	body, err := json.Marshal(pkgledger.SettlementResult{PaymentID: paymentID, Status: status})
	assert.NoError(t, err)
	return amqp.Delivery{Body: body, Headers: amqp.Table{"traceparent": "00-1234567890abcdef-12345678-01"}}
}

func withinTx(mockTx *mocks.MockTransactionRunner) {
	mockTx.EXPECT().
		WithinTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error {
			return fn(nil)
		})
}

// TestSettlementHandler_NormalFlow_Success verifica o caminho feliz: pagamento
// PENDING é atualizado para SETTLED e o notifier é chamado.
func TestSettlementHandler_NormalFlow_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockTx, mockRepo, mockAudit, mockNotifier := newHandler(ctrl)
	paymentID := uuid.New()

	withinTx(mockTx)
	mockRepo.EXPECT().
		FindByIDTx(gomock.Any(), gomock.Any(), paymentID).
		Return(&payment.Payment{ID: paymentID, Status: payment.StatusPending}, nil)
	mockRepo.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), paymentID, payment.StatusSettled, "").
		Return(nil)
	mockAudit.EXPECT().
		Insert(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)
	mockNotifier.EXPECT().
		NotifyPaymentStatus(gomock.Any(), paymentID, payment.StatusSettled, "").
		Return(nil)

	err := h.HandleMessage(context.Background(), settlementMsg(t, paymentID, "SETTLED"))
	assert.NoError(t, err)
}

// TestSettlementHandler_IdempotentBehavior verifica que mensagens duplicadas são
// silenciosamente ignoradas quando o status já foi atualizado anteriormente.
// O handler detecta isso ao ler o status atual antes de atualizar — sem depender
// de erros do banco de dados.
func TestSettlementHandler_IdempotentBehavior(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockTx, mockRepo, _, _ := newHandler(ctrl)
	paymentID := uuid.New()

	withinTx(mockTx)
	// Simula segunda entrega: status já é SETTLED (não PENDING).
	// UpdateStatus NÃO deve ser chamado — a mensagem é descartada.
	mockRepo.EXPECT().
		FindByIDTx(gomock.Any(), gomock.Any(), paymentID).
		Return(&payment.Payment{ID: paymentID, Status: payment.StatusSettled}, nil)

	err := h.HandleMessage(context.Background(), settlementMsg(t, paymentID, "SETTLED"))
	assert.NoError(t, err, "mensagem duplicada deve ser aceita silenciosamente")
}

// TestSettlementHandler_PaymentNotFound verifica que um payment inexistente
// gera um erro permanente (não vai para DLQ em loop).
func TestSettlementHandler_PaymentNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockTx, mockRepo, _, _ := newHandler(ctrl)
	paymentID := uuid.New()

	withinTx(mockTx)
	mockRepo.EXPECT().
		FindByIDTx(gomock.Any(), gomock.Any(), paymentID).
		Return(nil, sql.ErrNoRows)

	err := h.HandleMessage(context.Background(), settlementMsg(t, paymentID, "SETTLED"))
	assert.Error(t, err)
}

// TestSettlementHandler_DatabaseError_ShouldFail verifica que erros transientes
// de banco de dados propagam o erro para reprocessamento.
func TestSettlementHandler_DatabaseError_ShouldFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, mockTx, mockRepo, _, _ := newHandler(ctrl)
	paymentID := uuid.New()

	withinTx(mockTx)
	mockRepo.EXPECT().
		FindByIDTx(gomock.Any(), gomock.Any(), paymentID).
		Return(&payment.Payment{ID: paymentID, Status: payment.StatusPending}, nil)
	mockRepo.EXPECT().
		UpdateStatus(gomock.Any(), gomock.Any(), paymentID, payment.StatusSettled, "").
		Return(errors.New("connection timeout"))

	err := h.HandleMessage(context.Background(), settlementMsg(t, paymentID, "SETTLED"))
	assert.Error(t, err)
}

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
