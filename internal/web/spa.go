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

func spaHandler(fsys http.FileSystem) http.Handler {
	files := http.FileServer(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean("/" + r.URL.Path)

		if f, err := fsys.Open(strings.TrimPrefix(name, "/")); err == nil {
			defer f.Close()
			if info, statErr := f.Stat(); statErr == nil && !info.IsDir() {
				// Hashed asset filenames are immutable; index.html is not, and
				// caching it is how a browser ends up on last month's console.
				if strings.HasPrefix(name, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}

		// The history fallback is for routes the browser router owns, not for
		// everything. Asset names are content hashed, so a miss under /assets/
		// is a client asking for a build that no longer exists — answering with
		// index.html makes the browser parse HTML as JavaScript and report a
		// syntax error, hiding the real cause: a stale tab after a deploy.
		if strings.HasPrefix(name, "/assets/") {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		r2 := *r
		r2.URL.Path = "/"
		files.ServeHTTP(w, &r2)
	})
}
