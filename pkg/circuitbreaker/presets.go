package circuitbreaker

import (
	"log"
	"time"
)

func DefaultConfig() Config {
	return Config{
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from State, to State) {
			log.Printf("Circuit breaker '%s' changed from %s to %s", name, from.String(), to.String())
		},
	}
}

func HTTPConfig() Config {
	return Config{
		MaxRequests: 5,
		Interval:    30 * time.Second,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.5
		},
		OnStateChange: func(name string, from State, to State) {
			log.Printf("HTTP Circuit breaker '%s': %s -> %s", 
				name, from.String(), to.String())
		},
	}
}

func GRPCConfig() Config {
	return Config{
		MaxRequests: 3,
		Interval:    15 * time.Second,
		Timeout:     45 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 3 || 
				   (counts.Requests >= 5 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.4)
		},
		OnStateChange: func(name string, from State, to State) {
			log.Printf("gRPC Circuit breaker '%s': %s -> %s", name, from.String(), to.String())
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	}
}

func DatabaseConfig() Config {
	return Config{
		MaxRequests: 2,
		Interval:    20 * time.Second,
		Timeout:     90 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 5 || 
				   (counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.7)
		},
		OnStateChange: func(name string, from State, to State) {
			log.Printf("Database Circuit breaker '%s': %s -> %s", name, from.String(), to.String())
		},
	}
}

func MessagingConfig() Config {
	return Config{
		MaxRequests: 10,
		Interval:    15 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 8 ||
				   (counts.Requests >= 15 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.75)
		},
		OnStateChange: func(name string, from State, to State) {
			log.Printf("Messaging Circuit breaker '%s': %s -> %s", name, from.String(), to.String())
		},
	}
}