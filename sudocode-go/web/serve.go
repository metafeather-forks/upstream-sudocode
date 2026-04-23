// Package web serves the pre-built frontend SPA as embedded static assets.
package web

import (
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

//go:embed assets/*
var assets embed.FS

// assetsFS is the sub-filesystem rooted at "assets/".
var assetsFS, _ = fs.Sub(assets, "assets")

//encore:api public raw path=/web/*wildcard
func Serve(w http.ResponseWriter, req *http.Request) {
	// Strip the /web/ prefix to get the asset path.
	reqPath := strings.TrimPrefix(req.URL.Path, "/web/")
	if reqPath == "" || reqPath == "/" {
		reqPath = "index.html"
	}

	// Try to serve the requested file from embedded assets.
	if serveFile(w, reqPath) {
		return
	}

	// SPA fallback: serve index.html for non-asset paths.
	serveFile(w, "index.html")
}

// serveFile writes the named file from the embedded FS to w.
// Returns true if the file was found and served.
func serveFile(w http.ResponseWriter, name string) bool {
	f, err := assetsFS.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	// Check it's not a directory.
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	ext := filepath.Ext(name)
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f.(io.Reader))
	return true
}

// AssetFS exposes the embedded asset filesystem for testing.
func AssetFS() fs.FS {
	return assetsFS
}

// StripPrefix returns the asset path from a full request path.
func StripPrefix(requestPath string) string {
	p := strings.TrimPrefix(requestPath, "/web/")
	if p == "" {
		return "index.html"
	}
	return path.Clean(p)
}
