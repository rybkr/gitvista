//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/server"
)

// TestServerIntegration verifies the server starts, serves HTTP endpoints,
// and handles WebSocket connections using the current repository.
//
// Note: This test cannot run in parallel because the server uses http.DefaultServeMux
func TestServerIntegration(t *testing.T) {
	// Find the git repository (current working directory should be the repo root)
	repoPath, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Walk up to find .git directory
	for {
		gitDir := filepath.Join(repoPath, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			break
		}
		parent := filepath.Dir(repoPath)
		if parent == repoPath {
			t.Skip("not running in a git repository, skipping integration test")
		}
		repoPath = parent
	}

	// Create repository instance
	repo, err := gitcore.NewRepository(repoPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	// Create a minimal test FS for static files
	testFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}

	// Preflight: integration test needs to bind a local TCP listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local TCP listeners unavailable in this environment: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// Create and start server on an ephemeral localhost port to avoid collisions.
	srv := server.NewServer(repo, addr, testFS)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Check if server failed to start
	select {
	case err := <-errCh:
		t.Fatalf("server failed to start: %v", err)
	default:
	}

	baseURL := "http://" + addr

	// Cleanup
	defer srv.Shutdown()

	t.Run("health endpoint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("health check failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("health check status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var healthResp map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}

		if healthResp["status"] != "ok" {
			t.Errorf("health status = %q, want %q", healthResp["status"], "ok")
		}
	})

	t.Run("repository endpoint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/repository")
		if err != nil {
			t.Fatalf("repository request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var repoResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repoResp); err != nil {
			t.Fatalf("failed to decode repository response: %v", err)
		}

		if _, ok := repoResp["name"]; !ok {
			t.Error("response missing 'name' field")
		}
	})

	t.Run("graph summary endpoint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/graph/summary")
		if err != nil {
			t.Fatalf("graph summary request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var summary gitcore.GraphSummary
		if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
			t.Fatalf("failed to decode graph summary response: %v", err)
		}
		if summary.TotalCommits <= 0 {
			t.Errorf("summary totalCommits = %d, want > 0", summary.TotalCommits)
		}
	})

	t.Run("websocket connection", func(t *testing.T) {
		wsURL := "ws://" + addr + "/api/ws"
		wsOrigin := "http://" + addr

		// Connect to WebSocket
		headers := make(http.Header)
		headers.Set(textproto.CanonicalMIMEHeaderKey("Origin"), wsOrigin)
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
		if err != nil {
			t.Fatalf("websocket dial failed: %v (status: %v)", err, resp)
		}
		defer conn.Close()

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		// Read initial state message. Graph summary is fetched via HTTP bootstrap;
		// websocket carries lightweight live state on connect.
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read initial message: %v", err)
		}
		if messageType != websocket.TextMessage {
			t.Errorf("message type = %d, want %d (TextMessage)", messageType, websocket.TextMessage)
		}

		var msg struct {
			Summary *gitcore.GraphSummary `json:"summary"`
			Status  any                   `json:"status"`
			Head    any                   `json:"head"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			t.Fatalf("failed to unmarshal initial message: %v", err)
		}
		if msg.Summary != nil {
			t.Error("initial websocket message unexpectedly included summary")
		}
		if msg.Status == nil {
			t.Error("initial websocket message missing status")
		}
		if msg.Head == nil {
			t.Error("initial websocket message missing head")
		}

		// Send a ping to verify two-way communication
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			t.Errorf("failed to send ping: %v", err)
		}

		// The server should respond with a pong, but we won't wait for it
		// as the websocket library handles pong responses automatically
	})

	t.Run("invalid hash returns 400", func(t *testing.T) {
		// Create a fresh client to avoid rate limiting from previous tests
		time.Sleep(100 * time.Millisecond)
		resp, err := http.Get(baseURL + "/api/tree/invalid-hash")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		time.Sleep(100 * time.Millisecond)
		resp, err := http.Get(baseURL + "/api/tree/blame/abc123?path=../../etc/passwd")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("path traversal should return 400, got %d", resp.StatusCode)
		}
	})

	t.Run("rate limiting", func(t *testing.T) {
		// Wait for rate limiter to refill after previous tests
		time.Sleep(time.Second)

		client := &http.Client{Timeout: 2 * time.Second}

		// Make many requests quickly
		var successCount, rateLimitedCount int
		for i := 0; i < 200; i++ {
			resp, err := client.Get(baseURL + "/api/repository")
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				successCount++
			} else if resp.StatusCode == http.StatusTooManyRequests {
				rateLimitedCount++
			}
		}

		// We should have been rate limited at some point
		// The exact numbers depend on rate limit configuration
		if rateLimitedCount == 0 {
			t.Log("Warning: no requests were rate limited (may indicate rate limiting is disabled)")
		}

		t.Logf("Requests: %d successful, %d rate limited", successCount, rateLimitedCount)
	})
}

// TestServerShutdown verifies graceful shutdown works correctly.
func TestServerShutdown(t *testing.T) {
	// Find repository root
	repoPath, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		gitDir := filepath.Join(repoPath, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			break
		}
		parent := filepath.Dir(repoPath)
		if parent == repoPath {
			t.Skip("not running in a git repository, skipping integration test")
		}
		repoPath = parent
	}

	repo, err := gitcore.NewRepository(repoPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	testFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local TCP listeners unavailable in this environment: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	baseURL := "http://" + addr

	srv := server.NewServer(repo, addr, testFS)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-errCh:
		t.Fatalf("server failed to start: %v", err)
	default:
	}

	// Confirm server is reachable before shutdown.
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("health check before shutdown failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-shutdown health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	srv.Shutdown()

	// Server should stop accepting new connections.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	_, err = client.Get(baseURL + "/health")
	if err == nil {
		t.Fatal("expected request error after shutdown, got nil")
	}
}
