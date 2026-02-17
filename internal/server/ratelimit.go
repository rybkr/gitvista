package server

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a simple token bucket rate limiter per client IP.
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*bucket
	rate    int           // tokens per interval
	burst   int           // max tokens
	window  time.Duration // time window
}

// bucket represents a token bucket for a single client.
type bucket struct {
	tokens    int
	lastCheck time.Time
}

// newRateLimiter creates a rate limiter that allows 'rate' requests per 'window'
// with a burst capacity of 'burst'.
func newRateLimiter(rate int, burst int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		clients: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		window:  window,
	}

	// Cleanup goroutine to remove stale clients
	go rl.cleanup()

	return rl
}

// allow checks if a request from the given IP should be allowed.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, exists := rl.clients[ip]
	if !exists {
		b = &bucket{
			tokens:    rl.burst - 1,
			lastCheck: time.Now(),
		}
		rl.clients[ip] = b
		return true
	}

	now := time.Now()
	elapsed := now.Sub(b.lastCheck)

	// Add tokens based on elapsed time
	tokensToAdd := int(elapsed / rl.window * time.Duration(rl.rate))
	b.tokens += tokensToAdd
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastCheck = now

	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// cleanup removes clients that haven't made requests in 5 minutes.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, b := range rl.clients {
			if now.Sub(b.lastCheck) > 5*time.Minute {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitMiddleware wraps an http.HandlerFunc with rate limiting.
func (rl *rateLimiter) middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		if !rl.allow(ip) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// getClientIP extracts the client IP from the request.
// Checks X-Forwarded-For and X-Real-IP headers for proxied requests.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For (may contain multiple IPs)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	// Strip port if present
	ip := r.RemoteAddr
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}
