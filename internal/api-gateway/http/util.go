package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, ctx context.Context, log logger.Logger, code int, msg string) {
	log.Warn(ctx, "http error", "status", code, "msg", msg)
	writeJSON(w, code, map[string]string{"error": msg})
}

func handleGRPCError(w http.ResponseWriter, ctx context.Context, log logger.Logger, err error) {
	st, ok := status.FromError(err)
	if !ok {
		log.Error(ctx, "upstream call failed", "err", err)
		writeError(w, ctx, log, http.StatusBadGateway, "upstream error")
		return
	}
	switch st.Code() {
	case codes.InvalidArgument:
		writeError(w, ctx, log, http.StatusBadRequest, st.Message())
	case codes.NotFound:
		writeError(w, ctx, log, http.StatusNotFound, st.Message())
	case codes.DeadlineExceeded:
		writeError(w, ctx, log, http.StatusGatewayTimeout, "timeout")
	case codes.Unavailable:
		writeError(w, ctx, log, http.StatusServiceUnavailable, st.Message())
	default:
		log.Error(ctx, "upstream call failed", "code", st.Code().String(), "msg", st.Message())
		writeError(w, ctx, log, http.StatusBadGateway, st.Message())
	}
}

func parseCurrency(s string) (payment.Currency, bool) {
	switch strings.TrimSpace(strings.ToUpper(s)) {
	case "BRL", "CURRENCY_BRL":
		return payment.CurrencyBRL, true
	case "USD", "CURRENCY_USD":
		return payment.CurrencyUSD, true
	default:
		return "", false
	}
}
