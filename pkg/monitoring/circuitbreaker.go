package monitoring

import (
	"encoding/json"
	"net/http"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
	"github.com/LucasLCabral/payment-service/pkg/logger"
)

type CircuitBreakerProvider interface {
	CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts)
	CircuitBreakerName() string
}

type CircuitBreakerStatus struct {
	Name                 string  `json:"name"`
	State                string  `json:"state"`
	Requests             uint32  `json:"requests"`
	TotalSuccesses       uint32  `json:"total_successes"`
	TotalFailures        uint32  `json:"total_failures"`
	ConsecutiveSuccesses uint32  `json:"consecutive_successes"`
	ConsecutiveFailures  uint32  `json:"consecutive_failures"`
	SuccessRate          float64 `json:"success_rate"`
	FailureRate          float64 `json:"failure_rate"`
}

type Handler struct {
	log         logger.Logger
	cbProviders []CircuitBreakerProvider
}

func NewHandler(log logger.Logger) *Handler {
	return &Handler{
		log:         log,
		cbProviders: make([]CircuitBreakerProvider, 0),
	}
}

func (h *Handler) RegisterCircuitBreaker(provider CircuitBreakerProvider) {
	h.cbProviders = append(h.cbProviders, provider)
}

func (h *Handler) CircuitBreakerStatus(w http.ResponseWriter, r *http.Request) {
	statuses := make([]CircuitBreakerStatus, 0, len(h.cbProviders))

	for _, provider := range h.cbProviders {
		state, counts := provider.CircuitBreakerStats()

		var successRate, failureRate float64
		if counts.Requests > 0 {
			successRate = float64(counts.TotalSuccesses) / float64(counts.Requests) * 100
			failureRate = float64(counts.TotalFailures) / float64(counts.Requests) * 100
		}

		status := CircuitBreakerStatus{
			Name:                 provider.CircuitBreakerName(),
			State:                state.String(),
			Requests:             counts.Requests,
			TotalSuccesses:       counts.TotalSuccesses,
			TotalFailures:        counts.TotalFailures,
			ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
			ConsecutiveFailures:  counts.ConsecutiveFailures,
			SuccessRate:          successRate,
			FailureRate:          failureRate,
		}

		statuses = append(statuses, status)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"circuit_breakers": statuses,
		"timestamp":        r.Context().Value("request_time"),
	}); err != nil {
		h.log.Error(r.Context(), "failed to encode circuit breaker status", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": r.Context().Value("request_time"),
	}

	cbHealth := make(map[string]string)
	allHealthy := true

	for _, provider := range h.cbProviders {
		state, _ := provider.CircuitBreakerStats()
		cbHealth[provider.CircuitBreakerName()] = state.String()

		if state == circuitbreaker.StateOpen {
			allHealthy = false
		}
	}

	if len(cbHealth) > 0 {
		health["circuit_breakers"] = cbHealth
		if !allHealthy {
			health["status"] = "degraded"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		h.log.Error(r.Context(), "failed to encode health status", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}