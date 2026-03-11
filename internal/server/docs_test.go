package server

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
				{"id":"local","label":"Local Mode","title":"Local title","file":"local.md"}
			],
			"help":{"label":"Need More","title":"Help title","body":"Help body","primaryCta":{"label":"Open","href":"/"}}
		}`)},
		"hosted.md": {Data: []byte("Hosted paragraph.\n\n- first\n- second\n")},
		"local.md":  {Data: []byte("Local paragraph.\n")},
	}
}

func TestLoadDocsPage(t *testing.T) {
	page, err := loadDocsPage(testDocsFS())
	if err != nil {
		t.Fatalf("loadDocsPage() error = %v", err)
	}

	if page.Title != "Docs Title" {
		t.Fatalf("page title = %q, want %q", page.Title, "Docs Title")
	}
	if len(page.Sections) != 2 {
		t.Fatalf("len(page.Sections) = %d, want 2", len(page.Sections))
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
	s := newStaticTestServer(t, testWebFS())
	s.docsFS = testDocsFS()

	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	w := httptest.NewRecorder()

	s.handleDocs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body == "" || !strings.Contains(body, `"title":"Docs Title"`) || !strings.Contains(body, `"id":"hosted"`) {
		t.Fatalf("unexpected response body %q", body)
	}
}
