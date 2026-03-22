package hosted

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
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

type docsCTAResponse struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type docsSummaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type docsSectionResponse struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Title       string `json:"title"`
	Path        string `json:"path,omitempty"`
	ParentID    string `json:"parentId,omitempty"`
	Content     string `json:"content"`
	ContentHTML string `json:"contentHtml,omitempty"`
}

type docsHelpResponse struct {
	Label      string          `json:"label"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	PrimaryCTA docsCTAResponse `json:"primaryCta"`
}

type docsPageResponse struct {
	Eyebrow       string                `json:"eyebrow"`
	Title         string                `json:"title"`
	Lede          string                `json:"lede"`
	PrimaryCTA    docsCTAResponse       `json:"primaryCta"`
	SecondaryCTA  docsCTAResponse       `json:"secondaryCta"`
	Summary       []docsSummaryItem     `json:"summary"`
	Sections      []docsSectionResponse `json:"sections"`
	ActiveSection *docsSectionResponse  `json:"activeSection,omitempty"`
	Help          docsHelpResponse      `json:"help"`
}

func (h *Handler) HandleDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.DocsFS == nil {
		http.Error(w, "Docs are unavailable", http.StatusInternalServerError)
		return
	}

	requestedPath, err := sanitizeDocsPath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "Invalid docs path", http.StatusBadRequest)
		return
	}

	page, err := loadDocsPage(h.DocsFS, requestedPath)
	if err != nil {
		h.logger().Error("Failed to load docs", "err", err)
		http.Error(w, "Failed to load docs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(page); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func loadDocsPage(docsFS fs.FS, requestedPath string) (docsPageResponse, error) {
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
		rendered, err := loadDocsSectionFile(docsFS, section.File)
		if err != nil {
			return docsPageResponse{}, fmt.Errorf("load docs section %s: %w", section.File, err)
		}
		sectionResp := docsSectionResponse{
			ID:          section.ID,
			Label:       section.Label,
			Title:       section.Title,
			Path:        section.ID,
			Content:     rendered.Content,
			ContentHTML: rendered.ContentHTML,
		}
		sections = append(sections, sectionResp)
		if requestedPath == section.ID {
			selected := sectionResp
			page.ActiveSection = &selected
		}
	}
	page.Sections = sections

	if page.ActiveSection == nil && requestedPath != "" {
		nestedSection, found, err := loadNestedDocsSection(docsFS, meta.Sections, requestedPath)
		if err != nil {
			return docsPageResponse{}, err
		}
		if found {
			page.ActiveSection = &nestedSection
		}
	}

	return page, nil
}

type loadedDocsSection struct {
	Content     string
	ContentHTML string
}

func loadDocsSectionFile(docsFS fs.FS, name string) (loadedDocsSection, error) {
	body, err := fs.ReadFile(docsFS, name)
	if err != nil {
		return loadedDocsSection{}, err
	}
	if strings.HasSuffix(strings.ToLower(name), ".html") {
		return loadedDocsSection{
			Content:     "",
			ContentHTML: string(body),
		}, nil
	}

	contentHTML, err := renderDocsMarkdown(body)
	if err != nil {
		return loadedDocsSection{}, err
	}
	return loadedDocsSection{
		Content:     string(body),
		ContentHTML: contentHTML,
	}, nil
}

func loadNestedDocsSection(docsFS fs.FS, sections []docsSectionMeta, requestedPath string) (docsSectionResponse, bool, error) {
	for _, section := range sections {
		prefix := section.ID + "/"
		if !strings.HasPrefix(requestedPath, prefix) {
			continue
		}

		fileName, ok := resolveNestedDocsFile(docsFS, requestedPath)
		if !ok {
			return docsSectionResponse{}, false, nil
		}
		rendered, err := loadDocsSectionFile(docsFS, fileName)
		if err != nil {
			return docsSectionResponse{}, false, fmt.Errorf("load docs section %s: %w", fileName, err)
		}

		return docsSectionResponse{
			ID:          section.ID,
			Label:       section.Label,
			Title:       titleFromRenderedContent(rendered.ContentHTML, requestedPath),
			Path:        requestedPath,
			ParentID:    section.ID,
			Content:     rendered.Content,
			ContentHTML: rendered.ContentHTML,
		}, true, nil
	}

	return docsSectionResponse{}, false, nil
}

func resolveNestedDocsFile(docsFS fs.FS, requestedPath string) (string, bool) {
	candidates := []string{
		requestedPath,
		requestedPath + ".html",
		path.Join(requestedPath, "index.html"),
	}
	for _, candidate := range candidates {
		info, err := fs.Stat(docsFS, candidate)
		if err != nil || info.IsDir() {
			continue
		}
		return candidate, true
	}
	return "", false
}

var firstHeadingRE = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
var stripTagsRE = regexp.MustCompile(`(?s)<[^>]+>`)

func titleFromRenderedContent(html, fallback string) string {
	match := firstHeadingRE.FindStringSubmatch(html)
	if len(match) < 2 {
		return fallback
	}
	title := strings.TrimSpace(stripTagsRE.ReplaceAllString(match[1], ""))
	if title == "" {
		return fallback
	}
	return title
}

func sanitizeDocsPath(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	cleaned := path.Clean(strings.TrimSpace(raw))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == "" {
		return "", nil
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("path traversal")
	}
	if !fs.ValidPath(cleaned) {
		return "", fmt.Errorf("invalid path")
	}
	return cleaned, nil
}

var docsMarkdown = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
)

func renderDocsMarkdown(src []byte) (string, error) {
	var buf []byte
	var err error
	writer := &bufferWriter{buf: &buf}
	err = docsMarkdown.Convert(src, writer)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

type bufferWriter struct {
	buf *[]byte
}

func (w *bufferWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}
