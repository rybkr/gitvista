package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAccounts_ListAndCreate(t *testing.T) {
	s := newTestHostedServer(t)

	createReq := httptest.NewRequest("POST", "/api/accounts", strings.NewReader(`{"name":"Acme","slug":"acme"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.handleAccounts(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body: %s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created accountResponse
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	if created.Slug != "acme" {
		t.Fatalf("created slug = %q, want %q", created.Slug, "acme")
	}

	listReq := httptest.NewRequest("GET", "/api/accounts", nil)
	listW := httptest.NewRecorder()
	s.handleAccounts(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listW.Code, http.StatusOK)
	}

	var accounts []accountResponse
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
