package gitcore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// mailmapEntry represents a single mapping rule from a .mailmap file.
// See git-mailmap(5) for the full specification.
type mailmapEntry struct {
	properName  string
	properEmail string
	commitName  string
	commitEmail string
}

// Mailmap holds parsed .mailmap entries and resolves author identities.
type Mailmap struct {
	entries []mailmapEntry
}

// parseMailmap parses a .mailmap file's content into a Mailmap.
// It supports all four forms defined in git-mailmap(5):
//  1. Proper Name <commit@email>
//  2. <proper@email> <commit@email>
//  3. Proper Name <proper@email> <commit@email>
//  4. Proper Name <proper@email> Commit Name <commit@email>
func parseMailmap(content string) *Mailmap {
	m := &Mailmap{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if entry, ok := parseMailmapLine(line); ok {
			m.entries = append(m.entries, entry)
		}
	}
	return m
}

// parseMailmapLine parses a single .mailmap line into a mailmapEntry.
func parseMailmapLine(line string) (mailmapEntry, bool) {
	// Extract all <email> tokens and the text outside them.
	var emails []string
	var textParts []string
	remaining := line

	for {
		open := strings.IndexByte(remaining, '<')
		if open == -1 {
			textParts = append(textParts, remaining)
			break
		}
		close := strings.IndexByte(remaining[open:], '>')
		if close == -1 {
			// Malformed line — no closing bracket.
			return mailmapEntry{}, false
		}
		close += open

		textParts = append(textParts, remaining[:open])
		emails = append(emails, strings.TrimSpace(remaining[open+1:close]))
		remaining = remaining[close+1:]
	}

	if len(emails) == 0 {
		return mailmapEntry{}, false
	}

	// Build name parts from the text fragments between email tokens.
	names := make([]string, len(textParts))
	for i, t := range textParts {
		names[i] = strings.TrimSpace(t)
	}

	var entry mailmapEntry

	switch len(emails) {
	case 1:
		// Form 1: Proper Name <commit@email>
		entry.properName = names[0]
		entry.commitEmail = emails[0]
	case 2:
		name1 := names[0]
		name2 := names[1]

		if name1 == "" && name2 == "" {
			// Form 2: <proper@email> <commit@email>
			entry.properEmail = emails[0]
			entry.commitEmail = emails[1]
		} else if name2 == "" {
			// Form 3: Proper Name <proper@email> <commit@email>
			entry.properName = name1
			entry.properEmail = emails[0]
			entry.commitEmail = emails[1]
		} else {
			// Form 4: Proper Name <proper@email> Commit Name <commit@email>
			entry.properName = name1
			entry.properEmail = emails[0]
			entry.commitName = name2
			entry.commitEmail = emails[1]
		}
	default:
		return mailmapEntry{}, false
	}

	if entry.commitEmail == "" {
		return mailmapEntry{}, false
	}

	return entry, true
}

// resolve applies the mailmap to a Signature, replacing Name and/or Email
// with the canonical values. Matching is case-insensitive on email and, when
// specified, on the commit name. Last matching entry wins, per git semantics.
func (m *Mailmap) resolve(sig *Signature) {
	if m == nil || len(m.entries) == 0 {
		return
	}

	sigEmailLower := strings.ToLower(sig.Email)
	sigNameLower := strings.ToLower(sig.Name)

	for _, e := range m.entries {
		if strings.ToLower(e.commitEmail) != sigEmailLower {
			continue
		}
		if e.commitName != "" && strings.ToLower(e.commitName) != sigNameLower {
			continue
		}
		if e.properName != "" {
			sig.Name = e.properName
		}
		if e.properEmail != "" {
			sig.Email = e.properEmail
		}
	}
}

// loadMailmap reads and applies .mailmap from the working directory.
// It is a no-op if the file does not exist or if the repository is bare.
func (r *Repository) loadMailmap() error {
	if r.IsBare() {
		return nil
	}

	mailmapPath := filepath.Join(r.workDir, ".mailmap")
	//nolint:gosec // G304: Mailmap path is controlled by repository working directory
	data, err := os.ReadFile(mailmapPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	r.mailmap = parseMailmap(string(data))

	for _, c := range r.commits {
		r.mailmap.resolve(&c.Author)
		r.mailmap.resolve(&c.Committer)
	}
	for _, t := range r.tags {
		r.mailmap.resolve(&t.Tagger)
	}

	return nil
}
