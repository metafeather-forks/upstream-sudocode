package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServe_IndexHTML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/web/index.html", nil)
	w := httptest.NewRecorder()

	Serve(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Fatalf("expected index.html content, got: %s", body)
	}
}

func TestServe_RootRedirectsToIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/web/", nil)
	w := httptest.NewRecorder()

	Serve(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Fatalf("expected index.html content, got: %s", body)
	}
}

func TestServe_SPAFallback(t *testing.T) {
	// A path that doesn't correspond to a real asset should serve index.html.
	req := httptest.NewRequest(http.MethodGet, "/web/some/deep/route", nil)
	w := httptest.NewRecorder()

	Serve(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "<div id=\"root\">") {
		t.Fatalf("SPA fallback should serve index.html, got: %s", body)
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/web/", "index.html"},
		{"/web/index.html", "index.html"},
		{"/web/js/app.js", "js/app.js"},
		{"/web/some/deep/route", "some/deep/route"},
	}

	for _, tt := range tests {
		got := StripPrefix(tt.input)
		if got != tt.want {
			t.Errorf("StripPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAssetFS_ContainsIndex(t *testing.T) {
	afs := AssetFS()
	f, err := afs.Open("index.html")
	if err != nil {
		t.Fatalf("expected index.html in embedded assets: %v", err)
	}
	f.Close()
}
