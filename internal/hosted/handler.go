package hosted

import (
	"log/slog"

	"github.com/rybkr/gitvista/internal/repomanager"
)

type Handler struct {
	Logger        *slog.Logger
	Store         HostedStore
	RepoManager   *repomanager.RepoManager
	RemoveSession func(string)
}
