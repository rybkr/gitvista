package server

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(noopWriter{}, nil))
}

// newTestServer constructs a Server without calling Start(), leaving httpServer nil.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	repo := gitcore.NewEmptyRepository()
	webFS := os.DirFS(t.TempDir())
	s := NewServer(repo, "127.0.0.1:0", webFS)
	s.logger = silentLogger()
	return s
}

// TestShutdown_BeforeStart verifies that calling Shutdown() when httpServer is nil
// does not panic and returns promptly.
func TestShutdown_BeforeStart(t *testing.T) {
	s := newTestServer(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Shutdown()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown() blocked indefinitely when called before Start()")
	}
}

// TestShutdown_CancelsContext verifies that after Shutdown() the server's internal
// context is canceled.
func TestShutdown_CancelsContext(t *testing.T) {
	s := newTestServer(t)

	select {
	case <-s.ctx.Done():
		t.Fatal("context was already canceled before Shutdown()")
	default:
	}

	s.Shutdown()

	select {
	case <-s.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context was not canceled after Shutdown()")
	}
}

func TestShutdown_ClosesRateLimiterOnce(t *testing.T) {
	s := newTestServer(t)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Shutdown() panicked on second call (double-close of rateLimiter): %v", r)
		}
	}()

	s.Shutdown()
}

// TestShutdown_EmptyClientsMap verifies that Shutdown() with no connected WebSocket
// clients neither panics nor leaves the session's clients map in a nil state.
func TestShutdown_EmptyClientsMap(t *testing.T) {
	s := newTestServer(t)
	s.Shutdown()

	s.localSession.clientsMu.RLock()
	defer s.localSession.clientsMu.RUnlock()

	if s.localSession.clients == nil {
		t.Error("clients map is nil after Shutdown(); expected an initialized empty map")
	}
}

// TestShutdown_WaitGroupReachesZero verifies that the internal WaitGroup finishes
// after Shutdown() completes.
func TestShutdown_WaitGroupReachesZero(t *testing.T) {
	s := newTestServer(t)

	// Simulate what Start() does for the session's broadcast goroutine.
	s.localSession.Start()

	done := make(chan struct{})
	go func() {
		s.Shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown() did not complete within 5 s; WaitGroup may not have reached zero")
	}
}

// TestNewServer_InitialisesFields verifies that NewServer sets up all fields
// that Shutdown() depends on.
func TestNewServer_InitialisesFields(t *testing.T) {
	s := newTestServer(t)

	if s.ctx == nil {
		t.Error("ctx is nil after NewServer()")
	}
	if s.cancel == nil {
		t.Error("cancel is nil after NewServer()")
	}
	if s.rateLimiter == nil {
		t.Error("rateLimiter is nil after NewServer()")
	}
	if s.localSession == nil {
		t.Error("localSession is nil after NewServer()")
	}
	if s.localSession.clients == nil {
		t.Error("session clients map is nil after NewServer()")
	}
	if s.localSession.broadcast == nil {
		t.Error("session broadcast channel is nil after NewServer()")
	}
	if s.httpServer != nil {
		t.Error("httpServer should be nil before Start() is called")
	}
}

func TestHTTPServer_TimeoutConfiguration(t *testing.T) {
	addr := freePort(t)
	s := newTestServer(t)
	s.addr = addr

	startErr := make(chan error, 1)
	go func() {
		startErr <- s.Start()
	}()

	url := fmt.Sprintf("http://%s/health", addr)
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := httpGetNoKeepalive(url)
		if err == nil {
			resp.Body.Close()
			break
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil && time.Now().After(deadline) {
		s.Shutdown()
		t.Fatalf("server never responded on %s: %v", url, lastErr)
	}

	s.Shutdown()

	select {
	case err := <-startErr:
		if err != nil {
			t.Errorf("Start() returned unexpected error after Shutdown(): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start() did not return within 5 s of Shutdown() being called")
	}
}

func httpGetNoKeepalive(url string) (*http.Response, error) {
	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   2 * time.Second,
	}
	return client.Get(url) //nolint:noctx
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return fmt.Sprintf("127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
}

func TestShutdown_Concurrent(t *testing.T) {
	const goroutines = 4
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			s := newTestServer(t)
			s.Shutdown()
		}()
	}
	wg.Wait()
}
