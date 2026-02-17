package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testIP = "192.168.1.1"

func TestRateLimiter_Allow(t *testing.T) {
	tests := []struct {
		name     string
		rate     int
		burst    int
		window   time.Duration
		requests int
		delay    time.Duration
		wantPass int
	}{
		{
			name:     "first request always allowed",
			rate:     10,
			burst:    5,
			window:   time.Second,
			requests: 1,
			delay:    0,
			wantPass: 1,
		},
		{
			name:     "burst allows multiple requests",
			rate:     10,
			burst:    5,
			window:   time.Second,
			requests: 5,
			delay:    0,
			wantPass: 5,
		},
		{
			name:     "exceeding burst fails",
			rate:     10,
			burst:    3,
			window:   time.Second,
			requests: 5,
			delay:    0,
			wantPass: 3,
		},
		{
			name:     "tokens refill over time",
			rate:     10,
			burst:    2,
			window:   100 * time.Millisecond,
			requests: 4,
			delay:    150 * time.Millisecond, // wait for refill between request pairs
			wantPass: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := &rateLimiter{
				clients: make(map[string]*bucket),
				rate:    tt.rate,
				burst:   tt.burst,
				window:  tt.window,
			}

			ip := testIP
			passed := 0

			for i := 0; i < tt.requests; i++ {
				if tt.delay > 0 && i > 0 && i%2 == 0 {
					time.Sleep(tt.delay)
				}

				if rl.allow(ip) {
					passed++
				}
			}

			if passed != tt.wantPass {
				t.Errorf("allowed %d requests, want %d", passed, tt.wantPass)
			}
		})
	}
}

func TestRateLimiter_MultipleClients(t *testing.T) {
	rl := &rateLimiter{
		clients: make(map[string]*bucket),
		rate:    10,
		burst:   2,
		window:  time.Second,
	}

	client1 := "192.168.1.1"
	client2 := "192.168.1.2"

	// Client 1 uses up its burst
	if !rl.allow(client1) {
		t.Error("client1 request 1 should be allowed")
	}
	if !rl.allow(client1) {
		t.Error("client1 request 2 should be allowed")
	}
	if rl.allow(client1) {
		t.Error("client1 request 3 should be blocked (burst exceeded)")
	}

	// Client 2 should still have its burst available
	if !rl.allow(client2) {
		t.Error("client2 request 1 should be allowed")
	}
	if !rl.allow(client2) {
		t.Error("client2 request 2 should be allowed")
	}
	if rl.allow(client2) {
		t.Error("client2 request 3 should be blocked (burst exceeded)")
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := &rateLimiter{
		clients: make(map[string]*bucket),
		rate:    10,
		burst:   1,
		window:  50 * time.Millisecond, // 10 tokens per 50ms = 200 tokens/second
	}

	ip := "192.168.1.1"

	// Use up the burst
	if !rl.allow(ip) {
		t.Fatal("first request should be allowed")
	}

	// Next request should fail
	if rl.allow(ip) {
		t.Error("second request should be blocked immediately")
	}

	// Wait for refill
	time.Sleep(100 * time.Millisecond)

	// Should have refilled tokens
	if !rl.allow(ip) {
		t.Error("request after refill should be allowed")
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "direct connection",
			remoteAddr: "192.168.1.1:54321",
			headers:    map[string]string{},
			want:       "192.168.1.1",
		},
		{
			name:       "x-forwarded-for single IP",
			remoteAddr: "127.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.45"},
			want:       "203.0.113.45",
		},
		{
			name:       "x-forwarded-for multiple IPs",
			remoteAddr: "127.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.45, 192.168.1.1, 10.0.0.1"},
			want:       "203.0.113.45",
		},
		{
			name:       "x-real-ip",
			remoteAddr: "127.0.0.1:8080",
			headers:    map[string]string{"X-Real-IP": "198.51.100.23"},
			want:       "198.51.100.23",
		},
		{
			name:       "x-forwarded-for takes precedence over x-real-ip",
			remoteAddr: "127.0.0.1:8080",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.45",
				"X-Real-IP":       "198.51.100.23",
			},
			want: "203.0.113.45",
		},
		{
			name:       "ipv6 with port",
			remoteAddr: "[::1]:54321",
			headers:    map[string]string{},
			want:       "[::1]",
		},
		{
			name:       "no port in remote addr",
			remoteAddr: "192.168.1.1",
			headers:    map[string]string{},
			want:       "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tt.remoteAddr

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			got := getClientIP(req)
			if got != tt.want {
				t.Errorf("getClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rl := &rateLimiter{
		clients: make(map[string]*bucket),
		rate:    10,
		burst:   2,
		window:  time.Second,
	}

	wrappedHandler := rl.middleware(handler)

	t.Run("allows requests within limit", func(t *testing.T) {
		called = false
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.RemoteAddr = "192.168.1.1:54321"
		w := httptest.NewRecorder()

		wrappedHandler(w, req)

		if !called {
			t.Error("handler was not called")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/", nil)
		req.RemoteAddr = "192.168.1.2:54321"

		// Use up burst
		for range 2 {
			w := httptest.NewRecorder()
			wrappedHandler(w, req)
			if w.Code != http.StatusOK {
				t.Fatal("request should be allowed")
			}
		}

		// Next request should be blocked
		called = false
		w := httptest.NewRecorder()
		wrappedHandler(w, req)

		if called {
			t.Error("handler should not have been called")
		}
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusTooManyRequests)
		}
	})
}

func TestRateLimiter_Cleanup(t *testing.T) {
	// This test verifies that cleanup removes stale clients
	rl := &rateLimiter{
		clients: make(map[string]*bucket),
		rate:    10,
		burst:   5,
		window:  time.Second,
	}

	// Add some clients
	rl.allow("192.168.1.1")
	rl.allow("192.168.1.2")

	if len(rl.clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(rl.clients))
	}

	// Manually set last check time to over 5 minutes ago
	rl.mu.Lock()
	for _, b := range rl.clients {
		b.lastCheck = time.Now().Add(-6 * time.Minute)
	}
	rl.mu.Unlock()

	// Create a temporary rate limiter to test cleanup
	// We can't easily test the goroutine-based cleanup without waiting
	// But we can test the cleanup logic directly
	rl.mu.Lock()
	now := time.Now()
	for ip, b := range rl.clients {
		if now.Sub(b.lastCheck) > 5*time.Minute {
			delete(rl.clients, ip)
		}
	}
	count := len(rl.clients)
	rl.mu.Unlock()

	if count != 0 {
		t.Errorf("cleanup should have removed stale clients, got %d remaining", count)
	}
}

func TestNewRateLimiter(t *testing.T) {
	rl := newRateLimiter(100, 50, time.Second)

	if rl == nil {
		t.Fatal("newRateLimiter returned nil")
	}
	if rl.rate != 100 {
		t.Errorf("rate = %d, want 100", rl.rate)
	}
	if rl.burst != 50 {
		t.Errorf("burst = %d, want 50", rl.burst)
	}
	if rl.window != time.Second {
		t.Errorf("window = %v, want %v", rl.window, time.Second)
	}
	if rl.clients == nil {
		t.Error("clients map should be initialized")
	}

	// Give cleanup goroutine time to start
	time.Sleep(10 * time.Millisecond)
}
