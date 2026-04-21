package payment

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseCreatePaymentRequest(t *testing.T) {
	validID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	validPayer := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	validPayee := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	tests := []struct {
		name             string
		idempotencyKey   string
		amountCents      int64
		currency         Currency
		payerID          string
		payeeID          string
		description      string
		wantErr          bool
		wantIdempotency  uuid.UUID
		wantPayer        uuid.UUID
		wantPayee        uuid.UUID
		wantAmountCents  int64
		wantCurrency     Currency
		wantDescription  string
	}{
		{
			name:            "ok",
			idempotencyKey:  validID.String(),
			amountCents:     100,
			currency:        CurrencyBRL,
			payerID:         validPayer.String(),
			payeeID:         validPayee.String(),
			description:     "pix",
			wantErr:         false,
			wantIdempotency: validID,
			wantPayer:       validPayer,
			wantPayee:       validPayee,
			wantAmountCents: 100,
			wantCurrency:    CurrencyBRL,
			wantDescription: "pix",
		},
		{
			name:           "invalid idempotency",
			idempotencyKey: "not-a-uuid",
			amountCents:    1,
			currency:       CurrencyBRL,
			payerID:        validPayer.String(),
			payeeID:        validPayee.String(),
			wantErr:        true,
		},
		{
			name:           "invalid payer",
			idempotencyKey: validID.String(),
			amountCents:    1,
			currency:       CurrencyBRL,
			payerID:        "bad",
			payeeID:        validPayee.String(),
			wantErr:        true,
		},
		{
			name:           "invalid payee",
			idempotencyKey: validID.String(),
			amountCents:    1,
			currency:       CurrencyBRL,
			payerID:        validPayer.String(),
			payeeID:        "bad",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCreatePaymentRequest(
				tt.idempotencyKey,
				tt.amountCents,
				tt.currency,
				tt.payerID,
				tt.payeeID,
				tt.description,
			)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.IdempotencyKey != tt.wantIdempotency {
				t.Fatalf("IdempotencyKey = %v, want %v", got.IdempotencyKey, tt.wantIdempotency)
			}
			if got.PayerID != tt.wantPayer {
				t.Fatalf("PayerID = %v, want %v", got.PayerID, tt.wantPayer)
			}
			if got.PayeeID != tt.wantPayee {
				t.Fatalf("PayeeID = %v, want %v", got.PayeeID, tt.wantPayee)
			}
			if got.AmountCents != tt.wantAmountCents {
				t.Fatalf("AmountCents = %d, want %d", got.AmountCents, tt.wantAmountCents)
			}
			if got.Currency != tt.wantCurrency {
				t.Fatalf("Currency = %v, want %v", got.Currency, tt.wantCurrency)
			}
			if got.Description != tt.wantDescription {
				t.Fatalf("Description = %q, want %q", got.Description, tt.wantDescription)
			}
		})
	}
}

func TestCreatePaymentRequest_Validate(t *testing.T) {
	base := func() *CreatePaymentRequest {
		return &CreatePaymentRequest{
			IdempotencyKey: uuid.MustParse("11111111-1111-4111-8111-111111111111"),
			AmountCents:    50,
			Currency:       CurrencyUSD,
			PayerID:        uuid.MustParse("22222222-2222-4222-8222-222222222222"),
			PayeeID:        uuid.MustParse("33333333-3333-4333-8333-333333333333"),
			Description:    "ok",
		}
	}

	tests := []struct {
		name    string
		req     *CreatePaymentRequest
		wantErr bool
	}{
		{name: "nil", req: nil, wantErr: true},
		{name: "ok", req: base(), wantErr: false},
		{name: "amount zero", req: func() *CreatePaymentRequest { r := base(); r.AmountCents = 0; return r }(), wantErr: true},
		{name: "amount negative", req: func() *CreatePaymentRequest { r := base(); r.AmountCents = -1; return r }(), wantErr: true},
		{name: "invalid currency", req: func() *CreatePaymentRequest { r := base(); r.Currency = "EUR"; return r }(), wantErr: true},
		{name: "description too long", req: func() *CreatePaymentRequest {
			r := base()
			r.Description = strings.Repeat("a", 256)
			return r
		}(), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}
