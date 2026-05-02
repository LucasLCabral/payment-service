package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	ClientRPS   int
	ClientBurst int
	GlobalRPS   int
	GlobalBurst int
	SkipPaths []string
	CleanupInterval time.Duration
}


type RateLimiter struct {
	global    *rate.Limiter
	clients   map[string]*rate.Limiter
	mu        sync.Mutex
	skipPaths map[string]struct{}
	cfg       Config
}

type rateLimitError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewRateLimiter(cfg Config) *RateLimiter {
	skip := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skip[p] = struct{}{}
	}

	rl := &RateLimiter{
		global:    rate.NewLimiter(rate.Limit(cfg.GlobalRPS), cfg.GlobalBurst),
		clients:   make(map[string]*rate.Limiter),
		skipPaths: skip,
		cfg:       cfg,
	}

	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, skip := rl.skipPaths[r.URL.Path]; skip {
			next.ServeHTTP(w, r)
			return
		}

		if !rl.global.Allow() {
			rl.reject(w, http.StatusServiceUnavailable,
				"server_overloaded", "Server is temporarily overloaded. Please retry shortly.")
			return
		}

		if !rl.clientLimiter(clientIP(r)).Allow() {
			rl.reject(w, http.StatusTooManyRequests,
				"rate_limit_exceeded", "Too many requests. Slow down and retry.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) clientLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if l, ok := rl.clients[ip]; ok {
		return l
	}

	l := rate.NewLimiter(rate.Limit(rl.cfg.ClientRPS), rl.cfg.ClientBurst)
	rl.clients[ip] = l
	return l
}

func (rl *RateLimiter) cleanup() {
	interval := rl.cfg.CleanupInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, l := range rl.clients {
			if l.Tokens() == float64(rl.cfg.ClientBurst) {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) reject(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(rateLimitError{Code: code, Message: message})
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
