package gitcore

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// loadRefs loads all Git references (branches, tags) into the refs map.
func (r *Repository) loadRefs() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLooseRefs("heads"); err != nil {
		return fmt.Errorf("failed to load loose branches: %w", err)
	}
	if err := r.loadLooseRefs("tags"); err != nil {
		return fmt.Errorf("failed to load loose tags: %w", err)
	}
	if err := r.loadPackedRefs(); err != nil {
		return fmt.Errorf("failed to load packed refs: %w", err)
	}
	if err := r.loadHEAD(); err != nil {
		return fmt.Errorf("failed to load head: %w", err)
	}

	return nil
}

// loadLooseRefs recursively loads all refs in a directory.
// prefix is like "heads" for branches, or "tags" for tags.
func (r *Repository) loadLooseRefs(prefix string) error {
	refsDir := filepath.Join(r.gitDir, "refs", prefix)

	if _, err := os.Stat(refsDir); os.IsNotExist(err) {
		// No refs of this type yet (e.g., new repo with no tags), this is ok.
		return nil
	} else if err != nil {
		return err
	}

	return filepath.Walk(refsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(r.gitDir, path)
		if err != nil {
			return err
		}

		refName := filepath.ToSlash(relPath)
		hash, err := r.resolveRef(path)
		if err != nil {
			// Log the error but continue with other potentially valid refs.
			log.Printf("error resolving ref: %v", err)
			return nil
		}

		r.refs[refName] = hash
		return nil
	})
}

// loadPackedRefs reads the packed-refs file and loads all refs within.
func (r *Repository) loadPackedRefs() error {
	packedRefsFile := filepath.Join(r.gitDir, "packed-refs")

	//nolint:gosec // G304: Packed-refs path is controlled by git repository structure
	file, err := os.Open(packedRefsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close packed-refs file: %v", err)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		hash, err := NewHash(parts[0])
		if err != nil {
			continue
		}

		refName := parts[1]
		r.refs[refName] = hash
	}

	return scanner.Err()
}

// loadHEAD reads and caches HEAD information.
func (r *Repository) loadHEAD() error {
	headPath := filepath.Join(r.gitDir, "HEAD")
	//nolint:gosec // G304: HEAD path is controlled by git repository structure
	content, err := os.ReadFile(headPath)
	if err != nil {
		return fmt.Errorf("failed to read HEAD: %w", err)
	}

	line := strings.TrimSpace(string(content))

	if strings.HasPrefix(line, "ref: ") {
		r.headRef = strings.TrimPrefix(line, "ref: ")
		r.headDetached = false

		if hash, exists := r.refs[r.headRef]; exists {
			r.head = hash
		} else {
			r.head = "" // New repository with no commits, this is ok.
		}
	} else {
		r.headDetached = true
		r.headRef = ""

		hash, err := NewHash(line)
		if err != nil {
			return fmt.Errorf("invalid HEAD: %w", err)
		}
		r.head = hash
	}

	return nil
}

// loadStashes reads all stash entries from .git/logs/refs/stash (newest first).
// Returns an empty slice if no stashes exist.
func (r *Repository) loadStashes() []StashEntry {
	stashRefPath := filepath.Join(r.gitDir, "refs", "stash")
	if _, err := os.Stat(stashRefPath); os.IsNotExist(err) {
		return nil
	}

	// The stash reflog holds one entry per stash; iterate it newest-first.
	stashLogPath := filepath.Join(r.gitDir, "logs", "refs", "stash")
	//nolint:gosec // G304: Stash log path is controlled by git repository structure
	file, err := os.Open(stashLogPath)
	if err != nil {
		// Fallback: just the stash tip from refs/stash
		//nolint:gosec // G304: Stash ref path is controlled by git repository structure
		content, err := os.ReadFile(stashRefPath)
		if err != nil {
			return nil
		}
		hash, err := NewHash(strings.TrimSpace(string(content)))
		if err != nil {
			return nil
		}
		return []StashEntry{{Hash: hash, Message: "stash@{0}"}}
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close stash log: %v", err)
		}
	}()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Reflog is oldest-first; reverse for newest-first output.
	stashes := make([]StashEntry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Reflog format: <old-hash> <new-hash> <author info> <timestamp> <tz>\t<message>
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash, err := NewHash(parts[1])
		if err != nil {
			continue
		}
		msg := fmt.Sprintf("stash@{%d}", len(stashes))
		if tabIdx := strings.Index(line, "\t"); tabIdx >= 0 {
			msg = strings.TrimSpace(line[tabIdx+1:])
		}
		stashes = append(stashes, StashEntry{Hash: hash, Message: msg})
	}
	return stashes
}

// resolveRef reads a single ref file and returns its hash.
// Handles both direct hashes and symbolic refs.
func (r *Repository) resolveRef(path string) (Hash, error) {
	//nolint:gosec // G304: Ref paths are controlled by git repository structure
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(string(content))

	if strings.HasPrefix(line, "ref: ") {
		targetRef := strings.TrimPrefix(line, "ref: ")
		targetPath := filepath.Join(r.gitDir, targetRef)
		return r.resolveRef(targetPath)
	}

	hash, err := NewHash(line)
	if err != nil {
		return "", fmt.Errorf("invalid hash in ref file %s: %w", path, err)
	}
	return hash, nil
}
