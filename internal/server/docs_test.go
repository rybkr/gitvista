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
			"primaryCta":{"label":"Read","href":"#hosted"},
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
