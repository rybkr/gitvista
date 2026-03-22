package hosted

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testDocsFS() fstest.MapFS {
	return fstest.MapFS{
		"_meta.json": {Data: []byte(`{
			"eyebrow":"Product Docs",
			"title":"Docs Title",
			"lede":"Docs lede",
			"primaryCta":{"label":"Read","href":"/docs/hosted"},
			"secondaryCta":{"label":"Home","href":"/"},
			"summary":[{"label":"Hosted","value":"Browser"}],
			"sections":[
				{"id":"hosted","label":"Hosted Mode","title":"Hosted title","file":"hosted.md"},
				{"id":"local","label":"Local Mode","title":"Local title","file":"local.md"},
				{"id":"api","label":"API","title":"API title","file":"api/index.html"}
			],
			"help":{"label":"Need More","title":"Help title","body":"Help body","primaryCta":{"label":"Open","href":"/"}}
		}`)},
		"hosted.md":      {Data: []byte("Hosted paragraph.\n\n- first\n- second\n")},
		"local.md":       {Data: []byte("Local paragraph.\n")},
		"api/index.html": {Data: []byte("<h2 id=\"pkg-overview\">package gitvista</h2><p>API index.</p>")},
		"api/github.com/rybkr/gitvista/gitcore/index.html": {Data: []byte("<h2 id=\"pkg-overview\">package gitcore</h2><p>Gitcore API.</p>")},
	}
}

func TestLoadDocsPage(t *testing.T) {
	page, err := loadDocsPage(testDocsFS(), "")
	if err != nil {
		t.Fatalf("loadDocsPage() error = %v", err)
	}

	if page.Title != "Docs Title" {
		t.Fatalf("page title = %q, want %q", page.Title, "Docs Title")
	}
	if len(page.Sections) != 3 {
		t.Fatalf("len(page.Sections) = %d, want 3", len(page.Sections))
	}
	if page.Sections[0].Content == "" {
		t.Fatal("expected first section content to be populated")
	}
	if page.Sections[0].ContentHTML == "" {
		t.Fatal("expected first section html content to be populated")
	}
	if !strings.Contains(page.Sections[0].ContentHTML, "<p>Hosted paragraph.</p>") {
		t.Fatalf("expected rendered paragraph html, got %q", page.Sections[0].ContentHTML)
	}
	if !strings.Contains(page.Sections[0].ContentHTML, "<ul>") {
		t.Fatalf("expected rendered list html, got %q", page.Sections[0].ContentHTML)
	}
	if !strings.Contains(page.Sections[2].ContentHTML, "<h2 id=\"pkg-overview\">package gitvista</h2>") {
		t.Fatalf("expected embedded html content, got %q", page.Sections[2].ContentHTML)
	}
}

func TestLoadDocsPage_ActiveSection(t *testing.T) {
	page, err := loadDocsPage(testDocsFS(), "api/github.com/rybkr/gitvista/gitcore")
	if err != nil {
		t.Fatalf("loadDocsPage() error = %v", err)
	}

	if page.ActiveSection == nil {
		t.Fatal("expected active section to be populated")
	}
	if page.ActiveSection.ParentID != "api" {
		t.Fatalf("active section parent = %q, want %q", page.ActiveSection.ParentID, "api")
	}
	if page.ActiveSection.Path != "api/github.com/rybkr/gitvista/gitcore" {
		t.Fatalf("active section path = %q", page.ActiveSection.Path)
	}
	if page.ActiveSection.Title != "package gitcore" {
		t.Fatalf("active section title = %q, want %q", page.ActiveSection.Title, "package gitcore")
	}
}

func TestRenderDocsMarkdown(t *testing.T) {
	html, err := renderDocsMarkdown([]byte("# Title\n\nParagraph with `code`.\n\n- one\n  - two\n\n```bash\necho hi\n```\n"))
	if err != nil {
		t.Fatalf("renderDocsMarkdown() error = %v", err)
	}
	if !strings.Contains(html, "<h1 id=\"title\">Title</h1>") {
		t.Fatalf("expected heading in html, got %q", html)
	}
	if !strings.Contains(html, "<code>code</code>") {
		t.Fatalf("expected inline code in html, got %q", html)
	}
	if !strings.Contains(html, "<pre><code class=\"language-bash\">echo hi") {
		t.Fatalf("expected fenced code block in html, got %q", html)
	}
	if !strings.Contains(html, "<ul>") {
		t.Fatalf("expected list html, got %q", html)
	}
}

func TestHandleDocs(t *testing.T) {
	_, h := newTestHostedRuntime(t)
	h.DocsFS = testDocsFS()

	req := httptest.NewRequest(http.MethodGet, "/api/docs?path=api/github.com/rybkr/gitvista/gitcore", nil)
	w := httptest.NewRecorder()

	h.HandleDocs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body == "" || !strings.Contains(body, `"title":"Docs Title"`) || !strings.Contains(body, `"parentId":"api"`) {
		t.Fatalf("unexpected response body %q", body)
	}
}

func TestHandleDocs_InvalidPath(t *testing.T) {
	_, h := newTestHostedRuntime(t)
	h.DocsFS = testDocsFS()

	req := httptest.NewRequest(http.MethodGet, "/api/docs?path=../secret", nil)
	w := httptest.NewRecorder()

	h.HandleDocs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
