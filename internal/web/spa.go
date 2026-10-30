// The SPA handler is deliberately untagged: it takes a file system rather
// than the embedded one, so its behaviour — which paths fall back to the app
// and which are a plain 404 — is testable without a frontend build. Only the
// go:embed directive itself needs the embedui tag.
package web

import (
	"net/http"
	"path"
	"strings"
)

const appShell = "index.html"

func spaHandler(fsys http.FileSystem) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

		if f, ok := openRegular(fsys, name); ok {
			_ = f.Close()
			setCachePolicy(w, name)
			serveFile(w, r, fsys, name)
			return
		}

		// The history fallback is for routes the browser router owns, not for
		// everything. Asset names are content hashed, so a miss under /assets/
		// is a client asking for a build that no longer exists — answering with
		// index.html makes the browser parse HTML as JavaScript and report a
		// syntax error, hiding the real cause: a stale tab after a deploy.
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}

		setCachePolicy(w, appShell)
		serveFile(w, r, fsys, appShell)
	})
}

// setCachePolicy: hashed asset filenames are immutable; index.html is not, and
// caching it is how a browser ends up on last month's console. Anything else
// (the favicon, say) keeps the browser's own heuristics, as it always has.
func setCachePolicy(w http.ResponseWriter, name string) {
	switch {
	case strings.HasPrefix(name, "assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case name == appShell:
		w.Header().Set("Cache-Control", "no-cache")
	}
}

// serveFile answers with name, or with the precompressed copy the build wrote
// beside it when the client accepts that encoding. The console is ~0.9 MB of
// JavaScript before its first screen, and the server has no compression of its
// own; doing it once at build time costs nothing per request.
//
// Vary is set on every answer, compressed or not: the same URL now has several
// representations, and a shared cache that stored the plain one must not hand
// it to a client that asked for brotli, or the other way round.
func serveFile(w http.ResponseWriter, r *http.Request, fsys http.FileSystem, name string) {
	w.Header().Add("Vary", "Accept-Encoding")

	f, coding, err := openEncoded(fsys, name, r.Header.Get("Accept-Encoding"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, "unreadable file", http.StatusInternalServerError)
		return
	}
	if coding != "" {
		w.Header().Set("Content-Encoding", coding)
	}
	// ServeContent takes the Content-Type from the name it is given, so it is
	// given the original name: the .br copy is still JavaScript.
	http.ServeContent(w, r, name, info.ModTime(), f)
}
