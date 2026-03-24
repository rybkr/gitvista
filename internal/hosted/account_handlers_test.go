package hosted

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/server"
)

func TestHandleAccounts_ListAndCreate(t *testing.T) {
	_, h := newTestHostedRuntime(t)

	createReq := httptest.NewRequest("POST", "/api/accounts", strings.NewReader(`{"name":"Acme","slug":"acme"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	h.HandleAccounts(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body: %s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created AccountResponse
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.Slug != "acme" {
		t.Fatalf("created slug = %q, want %q", created.Slug, "acme")
	}

	listReq := httptest.NewRequest("GET", "/api/accounts", nil)
	listW := httptest.NewRecorder()
	h.HandleAccounts(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listW.Code, http.StatusOK)
	}

	var accounts []AccountResponse
	if err := json.NewDecoder(listW.Body).Decode(&accounts); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("account count = %d, want 2", len(accounts))
	}
	if !accounts[0].IsDefault {
		t.Fatalf("expected first account to be default")
	}
}

func newTestHostedRuntime(t *testing.T) (*server.Server, *Handler) {
	t.Helper()

	dataDir := t.TempDir()
	rm, err := repomanager.New(repomanager.Config{
		DataDir: dataDir,
		Logger:  silentLogger(),
	})
	if err != nil {
		t.Fatalf("failed to create repo manager: %v", err)
	}
	t.Cleanup(rm.Close)

	webFS := os.DirFS(t.TempDir())
	srv := server.NewFrontendServer("127.0.0.1:0", webFS, server.FrontendConfig{
		IndexPath:   "site/index.html",
		SPAFallback: true,
		ConfigMode:  "hosted",
	})
	h := NewHandler(srv, rm, NewMemoryHostedStore(rm))
	h.Logger = silentLogger()
	h.CacheSize = srv.CacheSize()
	return srv, h
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
