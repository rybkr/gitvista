package hosted

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type createAccountRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type AccountResponse struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	IsDefault bool      `json:"isDefault"`
	CreatedAt time.Time `json:"createdAt"`
}

func toAccountResponse(account HostedAccount) AccountResponse {
	return AccountResponse{
		ID:        account.ID,
		Slug:      account.Slug,
		Name:      account.Name,
		IsDefault: account.IsDefault,
		CreatedAt: account.CreatedAt,
	}
}

func (h *Handler) HandleAccounts(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.HandleListAccounts(w, r)
	case http.MethodPost:
		h.HandleCreateAccount(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) HandleListAccounts(w http.ResponseWriter, _ *http.Request) {
	accounts, err := h.Store.ListAccounts()
	if err != nil {
		http.Error(w, "Failed to list accounts", http.StatusInternalServerError)
		return
	}

	resp := make([]AccountResponse, 0, len(accounts))
	for _, account := range accounts {
		resp = append(resp, toAccountResponse(account))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger().Error("Failed to encode list-accounts response", "err", err)
	}
}

func (h *Handler) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	account, err := h.Store.CreateAccount(req.Name, req.Slug)
	if err != nil {
		statusCode := http.StatusBadRequest
		if !errors.Is(err, ErrHostedAccountNotFound) {
			statusCode = http.StatusBadRequest
		}
		http.Error(w, err.Error(), statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(toAccountResponse(account)); err != nil {
		h.logger().Error("Failed to encode create-account response", "err", err)
	}
}

func (h *Handler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}
