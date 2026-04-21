package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	stmocks "github.com/LucasLCabral/payment-service/internal/payment/settlement/mocks"
	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/mock/gomock"
)

type nopLog struct{}

func (nopLog) Debug(context.Context, string, ...any) {}
func (nopLog) Info(context.Context, string, ...any)  {}
func (nopLog) Warn(context.Context, string, ...any)  {}
func (nopLog) Error(context.Context, string, ...any) {}
func (nopLog) With(...any) logger.Logger              { return nopLog{} }

func TestMapStatus(t *testing.T) {
	tests := []struct {
		in   string
		want payment.PaymentStatus
	}{
		{"SETTLED", payment.StatusSettled},
		{"DECLINED", payment.StatusDeclined},
		{"", payment.StatusUnspecified},
		{"UNKNOWN", payment.StatusUnspecified},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := mapStatus(tt.in); got != tt.want {
				t.Fatalf("mapStatus(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestHandler_HandleMessage(t *testing.T) {
	payID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	traceID := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	headers := amqp.Table{trace.XTraceIDHeader: traceID.String()}

	tests := []struct {
		name         string
		body         []byte
		withNotifier bool
		setup        func(tx *stmocks.MockTransactionRunner, repo *stmocks.MockPayment, n *stmocks.MockPaymentStatusNotifier)
		wantErr      bool
		errSubstr    string
	}{
		{
			name:      "invalid json",
			body:      []byte(`{`),
			setup:     nil,
			wantErr:   true,
			errSubstr: "unmarshal",
		},
		{
			name:      "unknown settlement status",
			body:      mustJSON(t, pkgledger.SettlementResult{PaymentID: payID, Status: "WEIRD"}),
			setup:     nil,
			wantErr:   true,
			errSubstr: "unknown settlement status",
		},
		{
			name: "tx failed",
			body: mustJSON(t, pkgledger.SettlementResult{PaymentID: payID, Status: "SETTLED"}),
			setup: func(tx *stmocks.MockTransactionRunner, repo *stmocks.MockPayment, n *stmocks.MockPaymentStatusNotifier) {
				tx.EXPECT().WithinTransaction(gomock.Any(), gomock.Any()).Return(errors.New("begin failed"))
			},
			wantErr:   true,
			errSubstr: "begin failed",
		},
		{
			name: "update failed",
			body: mustJSON(t, pkgledger.SettlementResult{PaymentID: payID, Status: "SETTLED"}),
			setup: func(tx *stmocks.MockTransactionRunner, repo *stmocks.MockPayment, n *stmocks.MockPaymentStatusNotifier) {
				tx.EXPECT().WithinTransaction(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(*sql.Tx) error) error {
						return fn(nil)
					})
				repo.EXPECT().UpdateStatus(gomock.Any(), nil, payID, payment.StatusSettled, "").Return(errors.New("locked"))
			},
			wantErr:   true,
			errSubstr: "locked",
		},
		{
			name: "success without notifier",
			body: mustJSON(t, pkgledger.SettlementResult{PaymentID: payID, Status: "SETTLED"}),
			setup: func(tx *stmocks.MockTransactionRunner, repo *stmocks.MockPayment, n *stmocks.MockPaymentStatusNotifier) {
				tx.EXPECT().WithinTransaction(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(*sql.Tx) error) error {
						return fn(nil)
					})
				repo.EXPECT().UpdateStatus(gomock.Any(), nil, payID, payment.StatusSettled, "").Return(nil)
				repo.EXPECT().InsertAuditLog(gomock.Any(), nil, gomock.Any()).Return(nil)
			},
			wantErr: false,
		},
		{
			name:         "notifier error still ok",
			body:         mustJSON(t, pkgledger.SettlementResult{PaymentID: payID, Status: "DECLINED", DeclineReason: "nsf"}),
			withNotifier: true,
			setup: func(tx *stmocks.MockTransactionRunner, repo *stmocks.MockPayment, n *stmocks.MockPaymentStatusNotifier) {
				tx.EXPECT().WithinTransaction(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(*sql.Tx) error) error {
						return fn(nil)
					})
				repo.EXPECT().UpdateStatus(gomock.Any(), nil, payID, payment.StatusDeclined, "nsf").Return(nil)
				repo.EXPECT().InsertAuditLog(gomock.Any(), nil, gomock.Any()).Return(nil)
				n.EXPECT().NotifyPaymentStatus(gomock.Any(), payID, payment.StatusDeclined, "nsf").Return(errors.New("redis down"))
			},
			wantErr: false,
		},
		{
			name:         "notifier success",
			body:         mustJSON(t, pkgledger.SettlementResult{PaymentID: payID, Status: "SETTLED"}),
			withNotifier: true,
			setup: func(tx *stmocks.MockTransactionRunner, repo *stmocks.MockPayment, n *stmocks.MockPaymentStatusNotifier) {
				tx.EXPECT().WithinTransaction(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(*sql.Tx) error) error {
						return fn(nil)
					})
				repo.EXPECT().UpdateStatus(gomock.Any(), nil, payID, payment.StatusSettled, "").Return(nil)
				repo.EXPECT().InsertAuditLog(gomock.Any(), nil, gomock.Any()).Return(nil)
				n.EXPECT().NotifyPaymentStatus(gomock.Any(), payID, payment.StatusSettled, "").Return(nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mTx := stmocks.NewMockTransactionRunner(ctrl)
			mRepo := stmocks.NewMockPayment(ctrl)
			mNotifier := stmocks.NewMockPaymentStatusNotifier(ctrl)

			if tt.setup != nil {
				tt.setup(mTx, mRepo, mNotifier)
			}

			var notifier PaymentStatusNotifier
			if tt.withNotifier {
				notifier = mNotifier
			}

			h := NewHandler(mTx, mRepo, nopLog{}, notifier)
			msg := amqp.Delivery{Body: tt.body, Headers: headers}
			err := h.HandleMessage(context.Background(), msg)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("err %q should contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
