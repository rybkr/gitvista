package server

import (
	"net/http/httptest"
	"testing"
)

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
			name:       "untrusted remote ignores forwarding headers",
			remoteAddr: "203.0.113.10:8443",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.23"},
			want:       "203.0.113.10",
		},
		{
			name:       "ipv6 with port",
			remoteAddr: "[::1]:54321",
			headers:    map[string]string{},
			want:       "::1",
		},
		{
			name:       "no port in remote addr",
			remoteAddr: "192.168.1.1",
			headers:    map[string]string{},
			want:       "192.168.1.1",
		},
		{
			name:       "spoofed non-IP string in X-Forwarded-For falls through to RemoteAddr",
			remoteAddr: "192.168.1.1:54321",
			headers:    map[string]string{"X-Forwarded-For": "not-an-ip"},
			want:       "192.168.1.1",
		},
		{
			name:       "empty X-Forwarded-For falls through to RemoteAddr",
			remoteAddr: "192.168.1.1:54321",
			headers:    map[string]string{"X-Forwarded-For": ""},
			want:       "192.168.1.1",
		},
		{
			name:       "valid IPv6 in X-Forwarded-For",
			remoteAddr: "127.0.0.1:8080",
			headers:    map[string]string{"X-Forwarded-For": "2001:db8::1"},
			want:       "2001:db8::1",
		},
		{
			name:       "valid IPv6 in X-Real-IP",
			remoteAddr: "127.0.0.1:8080",
			headers:    map[string]string{"X-Real-IP": "2001:db8::1"},
			want:       "2001:db8::1",
		},
		{
			name:       "invalid X-Real-IP falls through to RemoteAddr",
			remoteAddr: "192.168.1.1:54321",
			headers:    map[string]string{"X-Real-IP": "not-an-ip"},
			want:       "192.168.1.1",
		},
		{
			name:       "IPv6 RemoteAddr with port returns bare address",
			remoteAddr: "[::1]:12345",
			headers:    map[string]string{},
			want:       "::1",
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
