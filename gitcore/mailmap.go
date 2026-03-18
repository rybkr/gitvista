package gitcore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type mailmapEntry struct {
	properName     string
	properEmail    string
	commitName     string
	commitEmail    string
	commitNameKey  string
	commitEmailKey string
}

// Mailmap holds parsed .mailmap entries and resolves author identities.
type Mailmap struct {
	entries        []mailmapEntry
	entriesByEmail map[string][]int
}

func parseMailmap(content string) *Mailmap {
	m := &Mailmap{
		entriesByEmail: make(map[string][]int),
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if entry, ok := parseMailmapLine(line); ok {
			entry.commitEmailKey = strings.ToLower(entry.commitEmail)
			if entry.commitName != "" {
				entry.commitNameKey = strings.ToLower(entry.commitName)
			}
			entryIdx := len(m.entries)
			m.entries = append(m.entries, entry)
			m.entriesByEmail[entry.commitEmailKey] = append(m.entriesByEmail[entry.commitEmailKey], entryIdx)
		}
	}
	return m
}

func parseMailmapLine(line string) (mailmapEntry, bool) {
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

	names := make([]string, len(textParts))
	for i, t := range textParts {
		names[i] = strings.TrimSpace(t)
	}

	var entry mailmapEntry
	switch len(emails) {
	case 1:
		entry.properName = names[0]
		entry.commitEmail = emails[0]
	case 2:
		name1 := names[0]
		name2 := names[1]

		if name1 == "" && name2 == "" {
			entry.properEmail = emails[0]
			entry.commitEmail = emails[1]
		} else if name2 == "" {
			entry.properName = name1
			entry.properEmail = emails[0]
			entry.commitEmail = emails[1]
		} else {
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

func (m *Mailmap) resolve(sig *Signature) {
	if m == nil || len(m.entries) == 0 {
		return
	}

	candidates := m.entriesByEmail[strings.ToLower(sig.Email)]
	if len(candidates) == 0 {
		return
	}

	sigNameLower := strings.ToLower(sig.Name)
	for _, idx := range candidates {
		e := m.entries[idx]
		if e.commitNameKey != "" && e.commitNameKey != sigNameLower {
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

func (r *Repository) loadMailmap() error {
	if r.IsBare() {
		return nil
	}

	mailmapPath := filepath.Join(r.workDir, ".mailmap")
	// #nosec G304 -- mailmap path is controlled by repository working directory
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
