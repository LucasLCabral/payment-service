package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpapi "github.com/LucasLCabral/payment-service/internal/api-gateway/http"
	"github.com/LucasLCabral/payment-service/internal/api-gateway/http/mocks"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type nopLogger struct{}

func (nopLogger) Debug(context.Context, string, ...any) {}
func (nopLogger) Info(context.Context, string, ...any)  {}
func (nopLogger) Warn(context.Context, string, ...any)  {}
func (nopLogger) Error(context.Context, string, ...any) {}
func (nopLogger) With(...any) logger.Logger              { return nopLogger{} }

func TestPaymentsHandler_Create(t *testing.T) {
	validID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	validPayer := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	validPayee := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	created := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		svc        httpapi.PaymentService
		body       string
		mock       func(ctrl *gomock.Controller) *mocks.MockPaymentService
		wantCode   int
		wantSubstr string
	}{
		{
			name:       "service unavailable",
			svc:        nil,
			body:       `{"idempotency_key":"` + validID.String() + `","amount_cents":100,"currency":"BRL","payer_id":"` + validPayer.String() + `","payee_id":"` + validPayee.String() + `"}`,
			mock:       nil,
			wantCode:   stdhttp.StatusServiceUnavailable,
			wantSubstr: "payment service unavailable",
		},
		{
			name: "invalid json",
			body: `{`,
			mock: func(ctrl *gomock.Controller) *mocks.MockPaymentService {
				return mocks.NewMockPaymentService(ctrl)
			},
			wantCode:   stdhttp.StatusBadRequest,
			wantSubstr: "invalid json",
		},
		{
			name: "invalid currency",
			body: `{"idempotency_key":"` + validID.String() + `","amount_cents":100,"currency":"EUR","payer_id":"` + validPayer.String() + `","payee_id":"` + validPayee.String() + `"}`,
			mock: func(ctrl *gomock.Controller) *mocks.MockPaymentService {
				return mocks.NewMockPaymentService(ctrl)
			},
			wantCode:   stdhttp.StatusBadRequest,
			wantSubstr: "currency must be BRL or USD",
		},
		{
			name: "invalid idempotency key",
			body: `{"idempotency_key":"x","amount_cents":100,"currency":"BRL","payer_id":"` + validPayer.String() + `","payee_id":"` + validPayee.String() + `"}`,
			mock: func(ctrl *gomock.Controller) *mocks.MockPaymentService {
				return mocks.NewMockPaymentService(ctrl)
			},
			wantCode:   stdhttp.StatusBadRequest,
			wantSubstr: "idempotency_key",
		},
		{
			name: "grpc invalid argument",
			body: `{"idempotency_key":"` + validID.String() + `","amount_cents":100,"currency":"BRL","payer_id":"` + validPayer.String() + `","payee_id":"` + validPayee.String() + `"}`,
			mock: func(ctrl *gomock.Controller) *mocks.MockPaymentService {
				m := mocks.NewMockPaymentService(ctrl)
				m.EXPECT().CreatePayment(gomock.Any(), gomock.AssignableToTypeOf(&payment.CreatePaymentRequest{})).
					Return(nil, status.Error(codes.InvalidArgument, "duplicate"))
				return m
			},
			wantCode:   stdhttp.StatusBadRequest,
			wantSubstr: "duplicate",
		},
		{
			name: "accepted",
			body: `{"idempotency_key":"` + validID.String() + `","amount_cents":100,"currency":"BRL","payer_id":"` + validPayer.String() + `","payee_id":"` + validPayee.String() + `"}`,
			mock: func(ctrl *gomock.Controller) *mocks.MockPaymentService {
				m := mocks.NewMockPaymentService(ctrl)
				pid := uuid.MustParse("44444444-4444-4444-8444-444444444444")
				m.EXPECT().CreatePayment(gomock.Any(), gomock.AssignableToTypeOf(&payment.CreatePaymentRequest{})).
					Return(&payment.Payment{
						ID:        pid,
						Status:    payment.StatusPending,
						CreatedAt: created,
					}, nil)
				return m
			},
			wantCode: stdhttp.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var svc httpapi.PaymentService
			if tt.mock != nil {
				svc = tt.mock(ctrl)
			} else if tt.svc != nil {
				svc = tt.svc
			}

			h := httpapi.NewPaymentsHandler(nopLogger{}, svc)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(stdhttp.MethodPost, "/payments", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			h.Create(rr, req)

			if rr.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body=%s", rr.Code, tt.wantCode, rr.Body.String())
			}
			if tt.wantSubstr != "" && !strings.Contains(rr.Body.String(), tt.wantSubstr) {
				t.Fatalf("body %q should contain %q", rr.Body.String(), tt.wantSubstr)
			}
			if tt.name == "accepted" {
				var out map[string]any
				if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if out["status"] != "PENDING" {
					t.Fatalf("status = %v", out["status"])
				}
			}
		})
	}
}

func TestPaymentsHandler_Get(t *testing.T) {
	pid := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	updated := time.Date(2026, 4, 21, 13, 0, 0, 0, time.UTC)
	created := updated.Add(-time.Hour)

	tests := []struct {
		name       string
		svc        httpapi.PaymentService
		pathID     string
		mock       func(ctrl *gomock.Controller) *mocks.MockPaymentService
		wantCode   int
		wantSubstr string
	}{
		{
			name:       "service unavailable",
			svc:        nil,
			pathID:     pid.String(),
			wantCode:   stdhttp.StatusServiceUnavailable,
			wantSubstr: "payment service unavailable",
		},
		{
			name:       "invalid id",
			pathID:     "not-uuid",
			mock:       func(ctrl *gomock.Controller) *mocks.MockPaymentService { return mocks.NewMockPaymentService(ctrl) },
			wantCode:   stdhttp.StatusBadRequest,
			wantSubstr: "invalid payment id",
		},
		{
			name:   "not found",
			pathID: pid.String(),
			mock: func(ctrl *gomock.Controller) *mocks.MockPaymentService {
				m := mocks.NewMockPaymentService(ctrl)
				m.EXPECT().GetPayment(gomock.Any(), gomock.Eq(&payment.GetPaymentRequest{PaymentID: pid})).
					Return(nil, status.Error(codes.NotFound, "missing"))
				return m
			},
			wantCode:   stdhttp.StatusNotFound,
			wantSubstr: "missing",
		},
		{
			name:   "ok",
			pathID: pid.String(),
			mock: func(ctrl *gomock.Controller) *mocks.MockPaymentService {
				m := mocks.NewMockPaymentService(ctrl)
				m.EXPECT().GetPayment(gomock.Any(), gomock.Eq(&payment.GetPaymentRequest{PaymentID: pid})).
					Return(&payment.Payment{
						ID:          pid,
						Status:      payment.StatusSettled,
						AmountCents: 100,
						Currency:    payment.CurrencyBRL,
						PayerID:     uuid.MustParse("22222222-2222-4222-8222-222222222222"),
						PayeeID:     uuid.MustParse("33333333-3333-4333-8333-333333333333"),
						CreatedAt:   created,
						UpdatedAt:   updated,
					}, nil)
				return m
			},
			wantCode: stdhttp.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var svc httpapi.PaymentService
			if tt.mock != nil {
				svc = tt.mock(ctrl)
			} else if tt.svc != nil {
				svc = tt.svc
			}

			h := httpapi.NewPaymentsHandler(nopLogger{}, svc)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(stdhttp.MethodGet, "/payments/"+tt.pathID, nil)
			req.SetPathValue("id", tt.pathID)

			h.Get(rr, req)

			if rr.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body=%s", rr.Code, tt.wantCode, rr.Body.String())
			}
			if tt.wantSubstr != "" && !strings.Contains(rr.Body.String(), tt.wantSubstr) {
				t.Fatalf("body %q should contain %q", rr.Body.String(), tt.wantSubstr)
			}
			if tt.name == "ok" {
				var out map[string]any
				if err := json.NewDecoder(bytes.NewReader(rr.Body.Bytes())).Decode(&out); err != nil {
					t.Fatal(err)
				}
				if out["status"] != "SETTLED" {
					t.Fatalf("status = %v", out["status"])
				}
			}
		})
	}
}
