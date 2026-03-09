package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
)

type docsMeta struct {
	Eyebrow      string            `json:"eyebrow"`
	Title        string            `json:"title"`
	Lede         string            `json:"lede"`
	PrimaryCTA   docsCTAResponse   `json:"primaryCta"`
	SecondaryCTA docsCTAResponse   `json:"secondaryCta"`
	Summary      []docsSummaryItem `json:"summary"`
	Sections     []docsSectionMeta `json:"sections"`
	Help         docsHelpMeta      `json:"help"`
}

type docsSectionMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Title string `json:"title"`
	File  string `json:"file"`
}

type docsHelpMeta struct {
	Label      string          `json:"label"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	PrimaryCTA docsCTAResponse `json:"primaryCta"`
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.docsFS == nil {
		http.Error(w, "Docs are unavailable", http.StatusInternalServerError)
		return
	}

	page, err := loadDocsPage(s.docsFS)
	if err != nil {
		s.logger.Error("Failed to load docs", "err", err)
		http.Error(w, "Failed to load docs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(page); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func loadDocsPage(docsFS fs.FS) (docsPageResponse, error) {
	var meta docsMeta
	metaBody, err := fs.ReadFile(docsFS, "_meta.json")
	if err != nil {
		return docsPageResponse{}, fmt.Errorf("read docs metadata: %w", err)
	}
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return docsPageResponse{}, fmt.Errorf("decode docs metadata: %w", err)
	}

	page := docsPageResponse{
		Eyebrow:      meta.Eyebrow,
		Title:        meta.Title,
		Lede:         meta.Lede,
		PrimaryCTA:   meta.PrimaryCTA,
		SecondaryCTA: meta.SecondaryCTA,
		Summary:      meta.Summary,
		Help: docsHelpResponse{
			Label:      meta.Help.Label,
			Title:      meta.Help.Title,
			Body:       meta.Help.Body,
			PrimaryCTA: meta.Help.PrimaryCTA,
		},
	}

	sections := make([]docsSectionResponse, 0, len(meta.Sections))
	for _, section := range meta.Sections {
		body, err := fs.ReadFile(docsFS, section.File)
		if err != nil {
			return docsPageResponse{}, fmt.Errorf("read docs section %s: %w", section.File, err)
		}
		sections = append(sections, docsSectionResponse{
			ID:      section.ID,
			Label:   section.Label,
			Title:   section.Title,
			Content: string(body),
		})
	}
	page.Sections = sections

	return page, nil
}
