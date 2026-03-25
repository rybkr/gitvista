package server

import (
	"net"
	"net/http"
	"strings"
)

// getClientIP extracts the client IP from the request.
// It only trusts X-Forwarded-For / X-Real-IP when the immediate peer appears
// to be a trusted proxy (loopback/private/link-local).
func getClientIP(r *http.Request) string {
	remoteIP, remoteRaw := remoteIP(r.RemoteAddr)
	if remoteIP == nil {
		return remoteRaw
	}

	if !isTrustedProxyIP(remoteIP) {
		return remoteIP.String()
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := strings.TrimSpace(xri)
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	return remoteIP.String()
}

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
