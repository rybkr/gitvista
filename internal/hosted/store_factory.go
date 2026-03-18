package hosted

import (
	"fmt"
	"strings"

	"github.com/rybkr/gitvista/internal/repomanager"
)

func NewHostedStore(rm *repomanager.RepoManager, databaseURL string) (HostedStore, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return NewMemoryHostedStore(rm), nil
	}

	store, err := NewPostgresHostedStore(databaseURL, rm)
	if err != nil {
		return nil, fmt.Errorf("create postgres hosted store: %w", err)
	}
	return store, nil
}
