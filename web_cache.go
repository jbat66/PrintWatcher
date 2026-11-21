package main

import "net/http"

// Mutable app assets must be fetched on refresh, even when a new container
// contains files whose modification times match the previous release.
func freshWebAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.html", "/app.js", "/style.css", "/manifest.json", "/login", "/setup":
			w.Header().Set("Cache-Control", "no-store")
			r = r.Clone(r.Context())
			r.Header.Del("If-Modified-Since")
			r.Header.Del("If-None-Match")
		}
		next.ServeHTTP(w, r)
	})
}
