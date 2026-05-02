package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock circuit breaker provider for testing
type mockCircuitBreakerProvider struct {
	name   string
	state  circuitbreaker.State
	counts circuitbreaker.Counts
}

func (m *mockCircuitBreakerProvider) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return m.state, m.counts
}

func (m *mockCircuitBreakerProvider) CircuitBreakerName() string {
	return m.name
}

func TestHandler_Health(t *testing.T) {
	log := logger.New("test")
	
	tests := []struct {
		name             string
		providers        []CircuitBreakerProvider
		expectedStatus   int
		expectedHealth   string
		expectedCBCount  int
	}{
		{
			name:           "no circuit breakers - healthy",
			providers:      []CircuitBreakerProvider{},
			expectedStatus: http.StatusOK,
			expectedHealth: "healthy",
			expectedCBCount: 0,
		},
		{
			name: "all circuit breakers closed - healthy",
			providers: []CircuitBreakerProvider{
				&mockCircuitBreakerProvider{
					name:  "service-a",
					state: circuitbreaker.StateClosed,
					counts: circuitbreaker.Counts{
						Requests:       100,
						TotalSuccesses: 95,
						TotalFailures:  5,
					},
				},
				&mockCircuitBreakerProvider{
					name:  "service-b", 
					state: circuitbreaker.StateClosed,
					counts: circuitbreaker.Counts{
						Requests:       50,
						TotalSuccesses: 50,
						TotalFailures:  0,
					},
				},
			},
			expectedStatus: http.StatusOK,
			expectedHealth: "healthy",
			expectedCBCount: 2,
		},
		{
			name: "some circuit breakers open - degraded",
			providers: []CircuitBreakerProvider{
				&mockCircuitBreakerProvider{
					name:  "service-a",
					state: circuitbreaker.StateClosed,
					counts: circuitbreaker.Counts{Requests: 10, TotalSuccesses: 10},
				},
				&mockCircuitBreakerProvider{
					name:  "service-b",
					state: circuitbreaker.StateOpen,
					counts: circuitbreaker.Counts{Requests: 10, TotalFailures: 10},
				},
			},
			expectedStatus: http.StatusPartialContent, // 206
			expectedHealth: "degraded",
			expectedCBCount: 2,
		},
		{
			name: "all circuit breakers open - unhealthy",
			providers: []CircuitBreakerProvider{
				&mockCircuitBreakerProvider{
					name:  "service-a",
					state: circuitbreaker.StateOpen,
					counts: circuitbreaker.Counts{Requests: 10, TotalFailures: 10},
				},
				&mockCircuitBreakerProvider{
					name:  "service-b",
					state: circuitbreaker.StateOpen,
					counts: circuitbreaker.Counts{Requests: 5, TotalFailures: 5},
				},
			},
			expectedStatus: http.StatusServiceUnavailable, // 503
			expectedHealth: "unhealthy",
			expectedCBCount: 2,
		},
		{
			name: "half-open circuit breaker - healthy",
			providers: []CircuitBreakerProvider{
				&mockCircuitBreakerProvider{
					name:  "service-recovery",
					state: circuitbreaker.StateHalfOpen,
					counts: circuitbreaker.Counts{Requests: 2, TotalSuccesses: 1, TotalFailures: 1},
				},
			},
			expectedStatus: http.StatusOK,
			expectedHealth: "healthy",
			expectedCBCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(log)
			
			for _, provider := range tt.providers {
				h.RegisterCircuitBreaker(provider)
			}

			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()
			
			h.Health(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedHealth, response["status"])
			assert.NotEmpty(t, response["timestamp"])
			assert.NotNil(t, response["uptime_seconds"])

			cbHealth, exists := response["circuit_breakers"]
			if tt.expectedCBCount == 0 {
				assert.True(t, exists)
				cbMap := cbHealth.(map[string]interface{})
				assert.Equal(t, 0, len(cbMap))
			} else {
				assert.True(t, exists)
				cbMap := cbHealth.(map[string]interface{})
				assert.Equal(t, tt.expectedCBCount, len(cbMap))

				for _, provider := range tt.providers {
					cbInfo, exists := cbMap[provider.CircuitBreakerName()]
					assert.True(t, exists)
					
					cbInfoMap := cbInfo.(map[string]interface{})
					assert.NotEmpty(t, cbInfoMap["state"])
					assert.NotNil(t, cbInfoMap["requests"])
					assert.NotNil(t, cbInfoMap["failures"])
					assert.NotNil(t, cbInfoMap["success_rate"])
				}
			}

			summary, exists := response["summary"]
			assert.True(t, exists)
			summaryMap := summary.(map[string]interface{})
			assert.Equal(t, float64(tt.expectedCBCount), summaryMap["total_circuit_breakers"])
		})
	}
}

func TestHandler_CircuitBreakerStatus(t *testing.T) {
	log := logger.New("test")
	handler := NewHandler(log)

	provider1 := &mockCircuitBreakerProvider{
		name:  "database",
		state: circuitbreaker.StateClosed,
		counts: circuitbreaker.Counts{
			Requests:             100,
			TotalSuccesses:       90,
			TotalFailures:        10,
			ConsecutiveSuccesses: 5,
			ConsecutiveFailures:  0,
		},
	}

	provider2 := &mockCircuitBreakerProvider{
		name:  "external-api",
		state: circuitbreaker.StateOpen,
		counts: circuitbreaker.Counts{
			Requests:             50,
			TotalSuccesses:       20,
			TotalFailures:        30,
			ConsecutiveSuccesses: 0,
			ConsecutiveFailures:  10,
		},
	}

	handler.RegisterCircuitBreaker(provider1)
	handler.RegisterCircuitBreaker(provider2)

	req := httptest.NewRequest("GET", "/circuit-breakers", nil)
	w := httptest.NewRecorder()

	handler.CircuitBreakerStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.NotEmpty(t, response["timestamp"])
	assert.Equal(t, float64(2), response["total_count"])

	cbList, exists := response["circuit_breakers"]
	assert.True(t, exists)
	
	cbArray := cbList.([]interface{})
	assert.Equal(t, 2, len(cbArray))

	cb1 := cbArray[0].(map[string]interface{})
	assert.Equal(t, "database", cb1["name"])
	assert.Equal(t, "CLOSED", cb1["state"])
	assert.Equal(t, float64(100), cb1["requests"])
	assert.Equal(t, float64(90), cb1["total_successes"])
	assert.Equal(t, float64(10), cb1["total_failures"])
	assert.Equal(t, float64(90), cb1["success_rate"]) // 90/100 * 100
	assert.Equal(t, float64(10), cb1["failure_rate"])  // 10/100 * 100

	cb2 := cbArray[1].(map[string]interface{})
	assert.Equal(t, "external-api", cb2["name"])
	assert.Equal(t, "OPEN", cb2["state"])
	assert.Equal(t, float64(50), cb2["requests"])
	assert.Equal(t, float64(20), cb2["total_successes"])
	assert.Equal(t, float64(30), cb2["total_failures"])
	assert.Equal(t, float64(40), cb2["success_rate"]) // 20/50 * 100
	assert.Equal(t, float64(60), cb2["failure_rate"])  // 30/50 * 100
}

func TestHandler_CircuitBreakerStatus_Empty(t *testing.T) {
	log := logger.New("test")
	handler := NewHandler(log)

	req := httptest.NewRequest("GET", "/circuit-breakers", nil)
	w := httptest.NewRecorder()

	handler.CircuitBreakerStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(0), response["total_count"])
	
	cbList := response["circuit_breakers"].([]interface{})
	assert.Equal(t, 0, len(cbList))
}

func TestHandler_RequestTimingMiddleware(t *testing.T) {
	log := logger.New("test")
	handler := NewHandler(log)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := r.Context().Value("request_start_time")
		assert.NotNil(t, startTime)
		
		start, ok := startTime.(time.Time)
		assert.True(t, ok)
		assert.True(t, time.Since(start) >= 0)
		
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := handler.RequestTimingMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_SuccessRateCalculation(t *testing.T) {
	tests := []struct {
		name            string
		totalSuccesses  uint32
		totalFailures   uint32
		expectedSuccess float64
		expectedFailure float64
	}{
		{
			name:            "perfect success rate",
			totalSuccesses:  100,
			totalFailures:   0,
			expectedSuccess: 100.0,
			expectedFailure: 0.0,
		},
		{
			name:            "perfect failure rate",
			totalSuccesses:  0,
			totalFailures:   100,
			expectedSuccess: 0.0,
			expectedFailure: 100.0,
		},
		{
			name:            "50-50 split",
			totalSuccesses:  50,
			totalFailures:   50,
			expectedSuccess: 50.0,
			expectedFailure: 50.0,
		},
		{
			name:            "no requests",
			totalSuccesses:  0,
			totalFailures:   0,
			expectedSuccess: 0.0,
			expectedFailure: 0.0,
		},
	}

	log := logger.New("test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(log)
			
			provider := &mockCircuitBreakerProvider{
				name:  "test-service",
				state: circuitbreaker.StateClosed,
				counts: circuitbreaker.Counts{
					Requests:       tt.totalSuccesses + tt.totalFailures,
					TotalSuccesses: tt.totalSuccesses,
					TotalFailures:  tt.totalFailures,
				},
			}
			
			handler.RegisterCircuitBreaker(provider)

			req := httptest.NewRequest("GET", "/circuit-breakers", nil)
			w := httptest.NewRecorder()

			handler.CircuitBreakerStatus(w, req)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			cbList := response["circuit_breakers"].([]interface{})
			cb := cbList[0].(map[string]interface{})

			assert.Equal(t, tt.expectedSuccess, cb["success_rate"])
			assert.Equal(t, tt.expectedFailure, cb["failure_rate"])
		})
	}
}