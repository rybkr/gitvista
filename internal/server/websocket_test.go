package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/gitcore"
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
			name:   "rejects alternate loopback host",
			host:   "127.0.0.1:8080",
			origin: "http://localhost:8080",
			want:   false,
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

func TestHostedUpgrader_CheckOrigin(t *testing.T) {
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
			got := hostedUpgrader.CheckOrigin(req)
			if got != tt.want {
				t.Errorf("hostedUpgrader.CheckOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleWebSocket_SendsBootstrapSequenceBeforeRegisteringClient(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	session := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
	})
	s := newTestServer(t)
	s.localSession = session

	handler := withLocalSession(session, s.handleWebSocket)
	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	wsURL := websocketURL(t, ts.URL+"/api/ws")
	header := http.Header{}
	header.Set("Origin", ts.URL)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	msg1 := readUpdateMessage(t, conn)
	if msg1.Type != messageTypeRepoSummary {
		t.Fatalf("first message type = %q, want %q", msg1.Type, messageTypeRepoSummary)
	}
	if msg1.Repo == nil {
		t.Fatal("first message missing repo payload")
	}

	msg2 := readUpdateMessage(t, conn)
	if msg2.Type != messageTypeBootstrapComplete {
		t.Fatalf("second message type = %q, want %q", msg2.Type, messageTypeBootstrapComplete)
	}

	msg3 := readUpdateMessage(t, conn)
	if msg3.Type != messageTypeStatus {
		t.Fatalf("third message type = %q, want %q", msg3.Type, messageTypeStatus)
	}

	msg4 := readUpdateMessage(t, conn)
	if msg4.Type != messageTypeHead {
		t.Fatalf("fourth message type = %q, want %q", msg4.Type, messageTypeHead)
	}

	session.clientsMu.RLock()
	clientCount := len(session.clients)
	session.clientsMu.RUnlock()
	if clientCount != 1 {
		t.Fatalf("registered clients = %d, want 1", clientCount)
	}
}

func TestHandleWebSocket_DoesNotReloadRepositoryDuringBootstrap(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	reloadCalls := 0
	session := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn: func() (*gitcore.Repository, error) {
			reloadCalls++
			return repo, nil
		},
		Logger: silentLogger(),
	})
	s := newTestServer(t)
	s.localSession = session

	handler := withLocalSession(session, s.handleWebSocket)
	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	wsURL := websocketURL(t, ts.URL+"/api/ws")
	header := http.Header{}
	header.Set("Origin", ts.URL)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	for i := 0; i < 4; i++ {
		_ = readUpdateMessage(t, conn)
	}
	if reloadCalls != 0 {
		t.Fatalf("reloadCalls = %d, want 0", reloadCalls)
	}
}

func websocketURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	return u.String()
}

func readUpdateMessage(t *testing.T, conn *websocket.Conn) UpdateMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var message UpdateMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	return message
}
