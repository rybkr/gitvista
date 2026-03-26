package server

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/gitcore"
)

func newWebSocketTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			if msg := fmt.Sprint(r); strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied") {
				t.Skipf("skipping websocket test in restricted environment: %s", msg)
			}
			panic(r)
		}
	}()
	return httptest.NewServer(handler)
}

func TestSameHostUpgrader_CheckOrigin(t *testing.T) {
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
			name:   "allows default https port omission",
			host:   "gitvista.io",
			origin: "https://gitvista.io:443",
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
			if strings.HasPrefix(tt.origin, "https://") {
				req.TLS = &tls.ConnectionState{}
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			got := sameHostUpgrader.CheckOrigin(req)
			if got != tt.want {
				t.Errorf("sameHostUpgrader.CheckOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSameHostUpgrader_CheckOrigin_WithForwardedProto(t *testing.T) {
	req := httptest.NewRequest("GET", "http://127.0.0.1/api/ws", nil)
	req.Host = "gitvista.io"
	req.Header.Set("Origin", "https://gitvista.io")
	req.Header.Set("X-Forwarded-Proto", "https")

	if !sameHostUpgrader.CheckOrigin(req) {
		t.Fatal("sameHostUpgrader.CheckOrigin() = false, want true")
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
	s.session = session

	handler := withSession(session, s.handleWebSocket)
	ts := newWebSocketTestServer(t, http.HandlerFunc(handler))
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

	waitForRegisteredClients(t, session, 1)
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
	s.session = session

	handler := withSession(session, s.handleWebSocket)
	ts := newWebSocketTestServer(t, http.HandlerFunc(handler))
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

func TestSendToAllClients_DeliversBroadcastPayload(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	session := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
	})
	s := newTestServer(t)
	s.session = session

	handler := withSession(session, s.handleWebSocket)
	ts := newWebSocketTestServer(t, http.HandlerFunc(handler))
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
	waitForRegisteredClients(t, session, 1)

	status := &WorkingTreeStatus{
		Modified: []FileStatus{{Path: "tracked.txt", StatusCode: "M"}},
	}
	session.sendToAllClients(UpdateMessage{Type: messageTypeStatus, Status: status})

	msg := readUpdateMessage(t, conn)
	if msg.Type != messageTypeStatus {
		t.Fatalf("message type = %q, want %q", msg.Type, messageTypeStatus)
	}
	if msg.Status == nil || len(msg.Status.Modified) != 1 || msg.Status.Modified[0].Path != "tracked.txt" {
		t.Fatalf("status payload = %+v", msg.Status)
	}
}

func TestBroadcastUpdate_DropsWhenChannelFull(t *testing.T) {
	rs := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: gitcore.NewEmptyRepository(),
		ReloadFn:    func() (*gitcore.Repository, error) { return gitcore.NewEmptyRepository(), nil },
		Logger:      silentLogger(),
	})

	for i := 0; i < broadcastChannelSize; i++ {
		rs.broadcast <- UpdateMessage{Type: messageTypeStatus}
	}

	rs.broadcastUpdate(UpdateMessage{Type: messageTypeHead, Head: &HeadInfo{Hash: "dropped"}})

	if got := len(rs.broadcast); got != broadcastChannelSize {
		t.Fatalf("broadcast queue length = %d, want %d", got, broadcastChannelSize)
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

func waitForRegisteredClients(t *testing.T, session *RepoSession, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session.clientsMu.RLock()
		got := len(session.clients)
		session.clientsMu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	session.clientsMu.RLock()
	got := len(session.clients)
	session.clientsMu.RUnlock()
	t.Fatalf("registered clients = %d, want %d", got, want)
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
