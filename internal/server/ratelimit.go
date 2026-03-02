package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	cleanupInterval  = 1 * time.Minute
	clientExpiration = 5 * time.Minute
)

// rateLimiter implements a simple token bucket rate limiter per client IP.
type rateLimiter struct {
	mu        sync.Mutex
	clients   map[string]*bucket
	rate      int           // tokens per interval
	burst     int           // max tokens
	window    time.Duration // time window
	stop      chan struct{}
	closeOnce sync.Once
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
		stop:    make(chan struct{}),
	}

	go rl.cleanup()

	return rl
}

// Close stops the cleanup goroutine. Call during server shutdown.
// Safe to call multiple times.
func (rl *rateLimiter) Close() {
	rl.closeOnce.Do(func() {
		close(rl.stop)
	})
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

	// Fix: use floating-point division to correctly compute fractional windows.
	// Integer division of elapsed/window always truncates to 0 for sub-window intervals.
	tokensToAdd := int(float64(elapsed) / float64(rl.window) * float64(rl.rate))
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

// cleanup removes clients that haven't made requests in clientExpiration and runs
// every cleanupInterval. Exits when Close() is called.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, b := range rl.clients {
				if now.Sub(b.lastCheck) > clientExpiration {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
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
// It only trusts X-Forwarded-For / X-Real-IP when the immediate peer appears
// to be a trusted proxy (loopback/private/link-local). This prevents direct
// clients from spoofing forwarding headers to bypass per-IP rate limits.
func getClientIP(r *http.Request) string {
	remoteIP, remoteRaw := remoteIP(r.RemoteAddr)
	if remoteIP == nil {
		return remoteRaw
	}

	if !isTrustedProxyIP(remoteIP) {
		return remoteIP.String()
	}

	// X-Forwarded-For may contain a comma-separated list of IPs; the first
	// entry is the original client. Validate before trusting it.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if net.ParseIP(ip) != nil {
			return ip
		}
		// Invalid value — fall through to next source.
	}

	// X-Real-IP should be a single IP; validate it.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := strings.TrimSpace(xri)
		if net.ParseIP(ip) != nil {
			return ip
		}
		// Invalid value — fall through to RemoteAddr.
	}

	return remoteIP.String()
}

// remoteIP extracts a parsed IP from RemoteAddr. It supports host:port and
// bare IP forms. If parsing fails, it returns nil and the raw value.
func remoteIP(remoteAddr string) (net.IP, string) {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip, host
		}
		return nil, remoteAddr
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip, remoteAddr
	}
	return nil, remoteAddr
}

func isTrustedProxyIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
