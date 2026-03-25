package server

import (
	"compress/flate"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 512
)

// sameHostUpgrader validates that the Origin header matches the request Host.
var sameHostUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return sameOriginHost(u, r)
	},
	EnableCompression: true,
}

func sameOriginHost(origin *url.URL, r *http.Request) bool {
	originHost, originPort := splitHostPort(origin.Host)
	requestHost, requestPort := splitHostPort(r.Host)
	if !strings.EqualFold(originHost, requestHost) {
		return false
	}

	originScheme := origin.Scheme
	if originScheme == "" {
		originScheme = effectiveRequestScheme(r)
	}
	requestScheme := effectiveRequestScheme(r)

	return defaultedPort(originPort, originScheme) == defaultedPort(requestPort, requestScheme)
}

func effectiveRequestScheme(r *http.Request) string {
	if proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); proto != "" {
		return strings.ToLower(proto)
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func splitHostPort(hostport string) (string, string) {
	if host, port, err := net.SplitHostPort(hostport); err == nil {
		return host, port
	}
	return hostport, ""
}

func defaultedPort(port string, scheme string) string {
	if port != "" {
		return port
	}
	switch strings.ToLower(scheme) {
	case "https", "wss":
		return "443"
	case "http", "ws":
		return "80"
	default:
		return ""
	}
}

// handleWebSocket upgrades the connection and delegates client management to
// the session extracted from the request context.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	session := SessionFromContext(r.Context())
	if session == nil {
		http.Error(w, "Repository not available", http.StatusInternalServerError)
		return
	}

	conn, err := sameHostUpgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed", "err", err)
		return
	}

	conn.EnableWriteCompression(true)
	if err := conn.SetCompressionLevel(flate.BestSpeed); err != nil {
		s.logger.Error("Failed to set compression level", "err", err)
	}
	conn.SetReadLimit(maxMessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		s.logger.Error("Failed to set read deadline", "addr", conn.RemoteAddr(), "err", err)
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	s.logger.Info("WebSocket client connected", "addr", conn.RemoteAddr())

	repo := session.Repo()
	if repo == nil {
		_ = conn.Close()
		s.logger.Error("WebSocket client bootstrap failed: repository not available", "addr", conn.RemoteAddr())
		return
	}

	// Send bootstrap payloads before registering for live broadcasts so the
	// client receives a coherent initial snapshot from cached repo state.
	if err := session.sendInitialRepoSummary(conn, buildRepositoryResponse(repo, r.Context())); err != nil {
		s.logger.Error("Failed to send initial repo summary", "addr", conn.RemoteAddr(), "err", err)
		_ = conn.Close()
		return
	}
	if err := session.sendInitialBootstrap(conn); err != nil {
		s.logger.Error("Failed to send initial bootstrap", "addr", conn.RemoteAddr(), "err", err)
		_ = conn.Close()
		return
	}

	writeMu := session.registerClient(conn)

	done := make(chan struct{})
	session.clientWg.Add(2)
	go session.clientReadPump(conn, done)
	go session.clientWritePump(conn, done, writeMu)
}

// WS lifecycle methods (sendInitialState, registerClient, removeClient,
// clientReadPump, clientWritePump) have been moved to RepoSession in session.go.
