package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rybkr/gitvista/gitcore"
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

// TestShutdown_EmptyClientsMap verifies that Shutdown() with no connected WebSocket
// clients neither panics nor leaves the session's clients map in a nil state.
func TestShutdown_EmptyClientsMap(t *testing.T) {
	s := newTestServer(t)
	s.Shutdown()

	s.session.clientsMu.RLock()
	defer s.session.clientsMu.RUnlock()

	if s.session.clients == nil {
		t.Error("clients map is nil after Shutdown(); expected an initialized empty map")
	}
}

// TestShutdown_WaitGroupReachesZero verifies that the internal WaitGroup finishes
// after Shutdown() completes.
func TestShutdown_WaitGroupReachesZero(t *testing.T) {
	s := newTestServer(t)

	// Simulate what Start() does for the session's broadcast goroutine.
	s.session.Start()

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
	if s.session == nil {
		t.Error("session is nil after NewServer()")
	}
	if s.session.clients == nil {
		t.Error("session clients map is nil after NewServer()")
	}
	if s.session.broadcast == nil {
		t.Error("session broadcast channel is nil after NewServer()")
	}
	if s.httpServer != nil {
		t.Error("httpServer should be nil before Start() is called")
	}
}

func TestNewHTTPServer_TimeoutAndHeaderConfiguration(t *testing.T) {
	s := newTestServer(t)
	handler := http.NewServeMux()

	srv := s.newHTTPServer(handler)

	if srv.ReadHeaderTimeout != readHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %s, want %s", srv.ReadHeaderTimeout, readHeaderTimeout)
	}
	if srv.MaxHeaderBytes != maxHeaderBytes {
		t.Errorf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, maxHeaderBytes)
	}
	if srv.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %s, want %s", srv.ReadTimeout, 15*time.Second)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %s, want %s", srv.WriteTimeout, 0*time.Second)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %s, want %s", srv.IdleTimeout, 120*time.Second)
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

func TestStart_FailedListen_CleansUpBackgroundState(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("skipping listener test in restricted environment: %v", err)
		}
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	s := newTestServer(t)
	s.addr = ln.Addr().String()

	startErr := s.Start()
	if startErr == nil {
		t.Fatal("Start() error = nil, want listen failure")
	}

	select {
	case <-s.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("server context was not canceled after failed Start()")
	}

	select {
	case <-s.session.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("session context was not canceled after failed Start()")
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
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("skipping listener test in restricted environment: %v", err)
		}
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

func TestServer_Start_UsesAddedRoutes(t *testing.T) {
	addr := freePort(t)
	webFS := os.DirFS(t.TempDir())
	s := newConfiguredServer(addr, webFS, AppConfig{
		IndexPath:   "app/index.html",
		SPAFallback: true,
	})
	s.logger = silentLogger()
	s.AddRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("/custom", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})

	startErr := make(chan error, 1)
	go func() {
		startErr <- s.Start()
	}()

	url := fmt.Sprintf("http://%s/custom", addr)
	deadline := time.Now().Add(5 * time.Second)
	var (
		resp    *http.Response
		lastErr error
	)
	for time.Now().Before(deadline) {
		resp, lastErr = httpGetNoKeepalive(url)
		if lastErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil {
		s.Shutdown()
		t.Fatalf("server never responded on %s: %v", url, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		s.Shutdown()
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	s.Shutdown()

	select {
	case err := <-startErr:
		if err != nil {
			t.Fatalf("Start() returned unexpected error after Shutdown(): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return within 5 s of Shutdown() being called")
	}
}
