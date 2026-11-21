package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWebAssetsRefreshDespiteCachedValidators(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"index.html", "app.js", "style.css", "manifest.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("new release"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := freshWebAssets(http.FileServer(http.Dir(dir)))
	for _, path := range []string{"/", "/app.js", "/style.css", "/manifest.json"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest("GET", path+"?v=old", nil)
			r.Header.Set("If-Modified-Since", "Wed, 01 Jan 2031 00:00:00 GMT")
			r.Header.Set("If-None-Match", "old")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Body.String() != "new release" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("stale asset response: %d %v %q", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}
