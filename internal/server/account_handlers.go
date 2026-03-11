package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type createAccountRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type accountResponse struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	IsDefault bool      `json:"isDefault"`
	CreatedAt time.Time `json:"createdAt"`
}

func toAccountResponse(account HostedAccount) accountResponse {
	return accountResponse{
		ID:        account.ID,
		Slug:      account.Slug,
		Name:      account.Name,
		IsDefault: account.IsDefault,
		CreatedAt: account.CreatedAt,
	}
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if s.hostedStore == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleListAccounts(w, r)
	case http.MethodPost:
		s.handleCreateAccount(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleListAccounts(w http.ResponseWriter, _ *http.Request) {
	accounts, err := s.hostedStore.ListAccounts()
	if err != nil {
		http.Error(w, "Failed to list accounts", http.StatusInternalServerError)
		return
	}

	resp := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		resp = append(resp, toAccountResponse(account))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("Failed to encode list-accounts response", "err", err)
	}
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	account, err := s.hostedStore.CreateAccount(req.Name, req.Slug)
	if err != nil {
		statusCode := http.StatusBadRequest
		if !errors.Is(err, errHostedAccountNotFound) {
			statusCode = http.StatusBadRequest
		}
		http.Error(w, err.Error(), statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(toAccountResponse(account)); err != nil {
		s.logger.Error("Failed to encode create-account response", "err", err)
	}
}
