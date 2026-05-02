package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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
	startTime   time.Time
}

func NewHandler(log logger.Logger) *Handler {
	return &Handler{
		log:         log,
		cbProviders: make([]CircuitBreakerProvider, 0),
		startTime:   time.Now().UTC(),
	}
}

func (h *Handler) RegisterCircuitBreaker(provider CircuitBreakerProvider) {
	h.cbProviders = append(h.cbProviders, provider)
}

func (h *Handler) RequestTimingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		ctx := context.WithValue(r.Context(), "request_start_time", start)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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

	response := map[string]interface{}{
		"circuit_breakers": statuses,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"total_count":      len(statuses),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.Error(r.Context(), "failed to encode circuit breaker status", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	status := "healthy"
	
	cbHealth := make(map[string]interface{})
	allHealthy := true
	openCount := 0
	totalCount := 0

	for _, provider := range h.cbProviders {
		state, counts := provider.CircuitBreakerStats()
		totalCount++
		
		cbInfo := map[string]interface{}{
			"state":        state.String(),
			"requests":     counts.Requests,
			"failures":     counts.TotalFailures,
			"success_rate": 0.0,
		}

		if counts.Requests > 0 {
			cbInfo["success_rate"] = float64(counts.TotalSuccesses) / float64(counts.Requests) * 100
		}

		cbHealth[provider.CircuitBreakerName()] = cbInfo

		if state == circuitbreaker.StateOpen {
			allHealthy = false
			openCount++
		}
	}

	if !allHealthy {
		if openCount == totalCount {
			status = "unhealthy"
		} else {
			status = "degraded"
		}
	}

	health := map[string]interface{}{
		"status":           status,
		"timestamp":        now.Format(time.RFC3339),
		"uptime_seconds":   int64(now.Sub(h.startTime).Seconds()),
		"circuit_breakers": cbHealth,
		"summary": map[string]interface{}{
			"total_circuit_breakers": totalCount,
			"open_circuit_breakers":  openCount,
			"healthy":                totalCount - openCount,
		},
	}

	statusCode := http.StatusOK
	switch status {
	case "unhealthy":
		statusCode = http.StatusServiceUnavailable
	case "degraded":
		statusCode = http.StatusPartialContent
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	if err := json.NewEncoder(w).Encode(health); err != nil {
		h.log.Error(r.Context(), "failed to encode health status", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}