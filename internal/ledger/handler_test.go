package ledger_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/LucasLCabral/payment-service/internal/ledger"
	ledgermocks "github.com/LucasLCabral/payment-service/internal/ledger/mocks"
	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
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

func TestHandler_HandleMessage(t *testing.T) {
	payID := uuid.MustParse("88888888-8888-4888-8888-888888888888")
	traceID := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	headers := amqp.Table{trace.XTraceIDHeader: traceID.String()}

	validBody, err := json.Marshal(pkgledger.PaymentCreatedEvent{
		Event:          "payment.created",
		PaymentID:      payID,
		IdempotencyKey: uuid.MustParse("99999999-9999-4999-8999-999999999999"),
		AmountCents:    100,
		Currency:       "BRL",
		PayerID:        uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		PayeeID:        uuid.MustParse("cccccccc-cccc-4ccc-8ccc-cccccccccccc"),
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		body      []byte
		headers   amqp.Table
		setup     func(m *ledgermocks.MockPaymentCreatedProcessor)
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "invalid json",
			body:      []byte(`{`),
			headers:   headers,
			setup:     nil,
			wantErr:   true,
			errSubstr: "unmarshal",
		},
		{
			name:    "processor error",
			body:    validBody,
			headers: headers,
			setup: func(m *ledgermocks.MockPaymentCreatedProcessor) {
				m.EXPECT().ProcessPaymentCreated(gomock.Any(), gomock.Any(), traceID).Return(errors.New("boom"))
			},
			wantErr:   true,
			errSubstr: "boom",
		},
		{
			name:    "ok",
			body:    validBody,
			headers: headers,
			setup: func(m *ledgermocks.MockPaymentCreatedProcessor) {
				m.EXPECT().ProcessPaymentCreated(gomock.Any(), gomock.Any(), traceID).Return(nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			proc := ledgermocks.NewMockPaymentCreatedProcessor(ctrl)
			if tt.setup != nil {
				tt.setup(proc)
			}

			h := ledger.NewHandler(proc, nopLog{})
			msg := amqp.Delivery{Body: tt.body, Headers: tt.headers}
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
