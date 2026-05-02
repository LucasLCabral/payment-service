package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_BasicBehavior(t *testing.T) {
	tests := []struct {
		name           string
		config         Config
		operations     []operation
		wantFinalState State
		wantCounts     expectedCounts
	}{
		{
			name:   "default behavior - starts closed",
			config: DefaultConfig(),
			operations: []operation{
				{action: "check_state", wantState: StateClosed},
				{action: "check_name", wantName: "test"},
			},
			wantFinalState: StateClosed,
		},
		{
			name:   "successful requests keep circuit closed",
			config: DefaultConfig(),
			operations: []operation{
				{action: "success", repeat: 5},
				{action: "check_state", wantState: StateClosed},
			},
			wantFinalState: StateClosed,
			wantCounts: expectedCounts{
				requests:       5,
				totalSuccesses: 5,
				totalFailures:  0,
			},
		},
		{
			name: "consecutive failures trip circuit",
			config: func() Config {
				cfg := DefaultConfig()
				cfg.ReadyToTrip = func(counts Counts) bool {
					return counts.ConsecutiveFailures >= 3
				}
				return cfg
			}(),
			operations: []operation{
				{action: "failure", repeat: 3, wantError: "test error"},
				{action: "check_state", wantState: StateOpen},
			},
			wantFinalState: StateOpen,
		},
		{
			name: "open state rejects requests",
			config: func() Config {
				cfg := DefaultConfig()
				cfg.ReadyToTrip = func(counts Counts) bool {
					return counts.ConsecutiveFailures >= 1
				}
				return cfg
			}(),
			operations: []operation{
				{action: "failure"},
				{action: "check_state", wantState: StateOpen},
				{action: "reject", wantError: ErrOpenState.Error()},
			},
			wantFinalState: StateOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker("test", tt.config)

			for i, op := range tt.operations {
				executeOperation(t, cb, op, i)
			}

			// Verify final state
			assert.Equal(t, tt.wantFinalState, cb.State())

			// Verify counts if specified
			if tt.wantCounts != (expectedCounts{}) {
				counts := cb.Counts()
				assert.Equal(t, tt.wantCounts.requests, counts.Requests, "requests count mismatch")
				assert.Equal(t, tt.wantCounts.totalSuccesses, counts.TotalSuccesses, "total successes mismatch")
				assert.Equal(t, tt.wantCounts.totalFailures, counts.TotalFailures, "total failures mismatch")
			}
		})
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		sequence []transitionStep
	}{
		{
			name: "half-open transition after timeout",
			config: func() Config {
				cfg := DefaultConfig()
				cfg.Timeout = 100 * time.Millisecond
				cfg.ReadyToTrip = func(counts Counts) bool {
					return counts.ConsecutiveFailures >= 1
				}
				return cfg
			}(),
			sequence: []transitionStep{
				{action: "failure", wantState: StateOpen},
				{action: "sleep", duration: 150 * time.Millisecond},
				{action: "check_state", wantState: StateHalfOpen},
			},
		},
		{
			name: "half-open to closed on success",
			config: func() Config {
				cfg := DefaultConfig()
				cfg.Timeout = 50 * time.Millisecond
				cfg.MaxRequests = 2
				cfg.ReadyToTrip = func(counts Counts) bool {
					return counts.ConsecutiveFailures >= 1
				}
				return cfg
			}(),
			sequence: []transitionStep{
				{action: "failure", wantState: StateOpen},
				{action: "sleep", duration: 100 * time.Millisecond},
				{action: "check_state", wantState: StateHalfOpen},
				{action: "success", repeat: 2},
				{action: "check_state", wantState: StateClosed},
			},
		},
		{
			name: "half-open to open on failure",
			config: func() Config {
				cfg := DefaultConfig()
				cfg.Timeout = 50 * time.Millisecond
				cfg.ReadyToTrip = func(counts Counts) bool {
					return counts.ConsecutiveFailures >= 1
				}
				return cfg
			}(),
			sequence: []transitionStep{
				{action: "failure", wantState: StateOpen},
				{action: "sleep", duration: 100 * time.Millisecond},
				{action: "check_state", wantState: StateHalfOpen},
				{action: "failure", wantState: StateOpen},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker("test", tt.config)

			for i, step := range tt.sequence {
				executeTransitionStep(t, cb, step, i)
			}
		})
	}
}

func TestCircuitBreaker_SpecialCases(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "context is passed correctly",
			testFunc: func(t *testing.T) {
				cb := NewCircuitBreaker("test", DefaultConfig())
				ctx := context.Background()

				err := cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
					require.NotNil(t, ctx)
					return nil
				})
				assert.NoError(t, err)
			},
		},
		{
			name: "panic handling updates circuit state",
			testFunc: func(t *testing.T) {
				config := DefaultConfig()
				config.ReadyToTrip = func(counts Counts) bool {
					return counts.ConsecutiveFailures >= 1
				}
				cb := NewCircuitBreaker("test", config)

				assert.Panics(t, func() {
					cb.Execute(func() error {
						panic("test panic")
					})
				})

				assert.Equal(t, StateOpen, cb.State())
			},
		},
		{
			name: "half-open state resets counts",
			testFunc: func(t *testing.T) {
				config := DefaultConfig()
				config.Timeout = 50 * time.Millisecond
				config.ReadyToTrip = func(counts Counts) bool {
					return counts.ConsecutiveFailures >= 1
				}
				cb := NewCircuitBreaker("test", config)

				// Trip circuit
				cb.Execute(func() error {
					return errors.New("failure")
				})
				assert.Equal(t, StateOpen, cb.State())

				// Wait for transition to half-open
				time.Sleep(100 * time.Millisecond)
				assert.Equal(t, StateHalfOpen, cb.State())

				// Counts should be reset
				counts := cb.Counts()
				assert.Equal(t, uint32(0), counts.Requests)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestPresetConfigurations(t *testing.T) {
	tests := []struct {
		name           string
		config         Config
		expectedFields configExpectations
	}{
		{
			name:   "Default",
			config: DefaultConfig(),
			expectedFields: configExpectations{
				hasReadyToTrip:    true,
				hasOnStateChange:  true,
				hasIsSuccessful:   false, // Default config doesn't set IsSuccessful
				maxRequestsSet:    true,
				timeoutSet:        true,
			},
		},
		{
			name:   "HTTP",
			config: HTTPConfig(),
			expectedFields: configExpectations{
				hasReadyToTrip:   true,
				hasOnStateChange: true,
				maxRequestsSet:   true,
				timeoutSet:       true,
			},
		},
		{
			name:   "gRPC",
			config: GRPCConfig(),
			expectedFields: configExpectations{
				hasReadyToTrip:   true,
				hasOnStateChange: true,
				hasIsSuccessful:  true,
				maxRequestsSet:   true,
				timeoutSet:       true,
			},
		},
		{
			name:   "Database",
			config: DatabaseConfig(),
			expectedFields: configExpectations{
				hasReadyToTrip:   true,
				hasOnStateChange: true,
				maxRequestsSet:   true,
				timeoutSet:       true,
			},
		},
		{
			name:   "Messaging",
			config: MessagingConfig(),
			expectedFields: configExpectations{
				hasReadyToTrip:   true,
				hasOnStateChange: true,
				maxRequestsSet:   true,
				timeoutSet:       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker("test-"+tt.name, tt.config)
			
			// Basic assertions
			assert.Equal(t, StateClosed, cb.State())
			assert.Equal(t, "test-"+tt.name, cb.Name())
			
			// Verify configuration was applied
			if tt.expectedFields.hasReadyToTrip {
				assert.NotNil(t, tt.config.ReadyToTrip, "ReadyToTrip should be set")
			}
			if tt.expectedFields.hasOnStateChange {
				assert.NotNil(t, tt.config.OnStateChange, "OnStateChange should be set")
			}
			if tt.expectedFields.hasIsSuccessful {
				assert.NotNil(t, tt.config.IsSuccessful, "IsSuccessful should be set")
			}
			if tt.expectedFields.maxRequestsSet {
				assert.Greater(t, tt.config.MaxRequests, uint32(0), "MaxRequests should be > 0")
			}
			if tt.expectedFields.timeoutSet {
				assert.Greater(t, tt.config.Timeout, time.Duration(0), "Timeout should be > 0")
			}
		})
	}
}

// Helper types and functions

type operation struct {
	action    string
	repeat    int
	wantError string
	wantState State
	wantName  string
}

type expectedCounts struct {
	requests       uint32
	totalSuccesses uint32
	totalFailures  uint32
}

type transitionStep struct {
	action    string
	repeat    int
	duration  time.Duration
	wantState State
}

type configExpectations struct {
	hasReadyToTrip   bool
	hasOnStateChange bool
	hasIsSuccessful  bool
	maxRequestsSet   bool
	timeoutSet       bool
}

func executeOperation(t *testing.T, cb CircuitBreaker, op operation, stepIndex int) {
	t.Helper()

	repeat := max(1, op.repeat)

	for i := range repeat {
		switch op.action {
		case "success":
			err := cb.Execute(func() error {
				return nil
			})
			assert.NoError(t, err, "step %d.%d: unexpected error on success", stepIndex, i)

		case "failure":
			testErr := errors.New("test error")
			err := cb.Execute(func() error {
				return testErr
			})
			if op.wantError != "" {
				assert.Error(t, err, "step %d.%d: expected error", stepIndex, i)
				assert.Contains(t, err.Error(), op.wantError, "step %d.%d: error message mismatch", stepIndex, i)
			} else {
				assert.Equal(t, testErr, err, "step %d.%d: error mismatch", stepIndex, i)
			}

		case "reject":
			err := cb.Execute(func() error {
				t.Fatalf("step %d.%d: function should not be called when circuit is open", stepIndex, i)
				return nil
			})
			assert.Error(t, err, "step %d.%d: expected rejection", stepIndex, i)
			if op.wantError != "" {
				assert.Contains(t, err.Error(), op.wantError, "step %d.%d: rejection error mismatch", stepIndex, i)
			}

		case "check_state":
			assert.Equal(t, op.wantState, cb.State(), "step %d: state mismatch", stepIndex)

		case "check_name":
			assert.Equal(t, op.wantName, cb.Name(), "step %d: name mismatch", stepIndex)
		}
	}
}

func executeTransitionStep(t *testing.T, cb CircuitBreaker, step transitionStep, stepIndex int) {
	t.Helper()

	switch step.action {
	case "success":
		repeat := max(1, step.repeat)
		for i := 0; i < repeat; i++ {
			err := cb.Execute(func() error {
				return nil
			})
			assert.NoError(t, err, "step %d.%d: unexpected error", stepIndex, i)
		}

	case "failure":
		err := cb.Execute(func() error {
			return errors.New("failure")
		})
		assert.Error(t, err, "step %d: expected error", stepIndex)
		if step.wantState != 0 {
			assert.Equal(t, step.wantState, cb.State(), "step %d: state after failure", stepIndex)
		}

	case "sleep":
		time.Sleep(step.duration)

	case "check_state":
		assert.Equal(t, step.wantState, cb.State(), "step %d: state mismatch", stepIndex)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}