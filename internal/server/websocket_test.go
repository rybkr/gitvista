package server

import (
	"net/http/httptest"
	"testing"
)

func TestLocalUpgrader_CheckOrigin(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{
			name:   "allows same host origin",
			host:   "127.0.0.1:8080",
			origin: "http://127.0.0.1:8080",
			want:   true,
		},
		{
			name:   "allows localhost origin",
			host:   "127.0.0.1:8080",
			origin: "http://localhost:8080",
			want:   true,
		},
		{
			name:   "allows loopback ipv6 origin",
			host:   "[::1]:8080",
			origin: "http://[::1]:8080",
			want:   true,
		},
		{
			name:   "rejects cross-site origin",
			host:   "127.0.0.1:8080",
			origin: "https://evil.example",
			want:   false,
		},
		{
			name:   "rejects missing origin",
			host:   "127.0.0.1:8080",
			origin: "",
			want:   false,
		},
		{
			name:   "rejects malformed origin",
			host:   "127.0.0.1:8080",
			origin: "://bad",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/api/ws", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			got := localUpgrader.CheckOrigin(req)
			if got != tt.want {
				t.Errorf("localUpgrader.CheckOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaaSUpgrader_CheckOrigin(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{
			name:   "allows same host origin",
			host:   "app.example.com",
			origin: "https://app.example.com",
			want:   true,
		},
		{
			name:   "rejects different host origin",
			host:   "app.example.com",
			origin: "https://evil.example",
			want:   false,
		},
		{
			name:   "rejects missing origin",
			host:   "app.example.com",
			origin: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/api/ws", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			got := saasUpgrader.CheckOrigin(req)
			if got != tt.want {
				t.Errorf("saasUpgrader.CheckOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
