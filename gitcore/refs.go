package gitcore

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	filepathWalk = filepath.Walk
	filepathRel  = filepath.Rel
	filepathAbs  = filepath.Abs
)

func (r *Repository) loadRefs() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Precedence must match git semantics: loose refs override packed refs.
	// Load packed first, then overlay loose refs.
	if err := r.loadPackedRefs(); err != nil {
		return fmt.Errorf("failed to load packed refs: %w", err)
	}
	if err := r.loadLooseRefs("heads"); err != nil {
		return fmt.Errorf("failed to load loose branches: %w", err)
	}
	if err := r.loadLooseRefs("remotes"); err != nil {
		return fmt.Errorf("failed to load loose remote-tracking refs: %w", err)
	}
	if err := r.loadLooseRefs("tags"); err != nil {
		return fmt.Errorf("failed to load loose tags: %w", err)
	}
	if err := r.loadHEAD(); err != nil {
		return fmt.Errorf("failed to load head: %w", err)
	}

	return nil
}

func (r *Repository) loadLooseRefs(prefix string) error {
	refsDir := filepath.Join(r.gitDir, "refs", prefix)

	if _, err := os.Stat(refsDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	var refErrs []error
	walkErr := filepathWalk(refsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepathRel(r.gitDir, path)
		if err != nil {
			return err
		}

		refName := filepath.ToSlash(relPath)
		hash, err := r.resolveRef(path)
		if err != nil {
			refErrs = append(refErrs, fmt.Errorf("resolving %s: %w", refName, err))
			return nil
		}

		r.refs[refName] = hash
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	return errors.Join(refErrs...)
}

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
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	var parseErrs []error
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			parseErrs = append(parseErrs, fmt.Errorf("invalid packed-refs line: %q", line))
			continue
		}

		hash, err := NewHash(parts[0])
		if err != nil {
			parseErrs = append(parseErrs, fmt.Errorf("invalid packed ref hash %q: %w", parts[0], err))
			continue
		}

		refName := parts[1]
		// Keep existing values (typically from loose refs) if present.
		if _, exists := r.refs[refName]; !exists {
			r.refs[refName] = hash
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return errors.Join(parseErrs...)
}

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
			r.head = "" // New repository with no commits yet
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

func (r *Repository) loadStashes() error {
	stashRefPath := filepath.Join(r.gitDir, "refs", "stash")
	if _, err := os.Stat(stashRefPath); os.IsNotExist(err) {
		return nil
	}

	stashLogPath := filepath.Join(r.gitDir, "logs", "refs", "stash")
	//nolint:gosec // G304: Stash log path is controlled by git repository structure
	file, err := os.Open(stashLogPath)
	if err != nil {
		// Fallback: just the stash tip from refs/stash
		//nolint:gosec // G304: Stash ref path is controlled by git repository structure
		content, err := os.ReadFile(stashRefPath)
		if err != nil {
			return fmt.Errorf("reading stash ref fallback: %w", err)
		}
		hash, err := NewHash(strings.TrimSpace(string(content)))
		if err != nil {
			return fmt.Errorf("parsing stash ref fallback: %w", err)
		}
		r.stashes = append(r.stashes, &StashEntry{
			Hash:    hash,
			Message: "stash@{0}",
		})
		return nil
	}
	defer func() { _ = file.Close() }()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Reflog is oldest-first; reverse for newest-first output
	var parseErrs []error
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			parseErrs = append(parseErrs, fmt.Errorf("invalid stash reflog line: %q", line))
			continue
		}
		hash, err := NewHash(parts[1])
		if err != nil {
			parseErrs = append(parseErrs, fmt.Errorf("invalid stash reflog hash %q: %w", parts[1], err))
			continue
		}
		msg := fmt.Sprintf("stash@{%d}", len(r.stashes))
		if tabIdx := strings.Index(line, "\t"); tabIdx >= 0 {
			msg = strings.TrimSpace(line[tabIdx+1:])
		}
		r.stashes = append(r.stashes, &StashEntry{
			Hash:    hash,
			Message: msg,
		})
	}

	return errors.Join(parseErrs...)
}

func (r *Repository) resolveRef(path string) (Hash, error) {
	return r.resolveRefDepth(path, 0)
}

func (r *Repository) resolveRefDepth(path string, depth int) (Hash, error) {
	const maxSymrefDepth = 10
	if depth > maxSymrefDepth {
		return "", fmt.Errorf("symbolic ref chain too deep (possible cycle) at: %s", path)
	}
	if err := ensurePathWithinBase(r.gitDir, path); err != nil {
		return "", fmt.Errorf("invalid ref path %q: %w", path, err)
	}

	// #nosec G304 -- path is constrained to r.gitDir by ensurePathWithinBase above.
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(string(content))

	if strings.HasPrefix(line, "ref: ") {
		targetRef := strings.TrimPrefix(line, "ref: ")
		if hash, exists := r.refs[targetRef]; exists {
			return hash, nil
		}
		targetPath := filepath.Join(r.gitDir, filepath.Clean(targetRef))
		return r.resolveRefDepth(targetPath, depth+1)
	}

	hash, err := NewHash(line)
	if err != nil {
		return "", fmt.Errorf("invalid hash in ref file %s: %w", path, err)
	}
	return hash, nil
}

func ensurePathWithinBase(base, candidate string) error {
	absBase, err := filepathAbs(base)
	if err != nil {
		return err
	}
	absCandidate, err := filepathAbs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepathRel(absBase, absCandidate)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes base directory")
	}
	return nil
}
