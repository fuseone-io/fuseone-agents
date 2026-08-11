//go:build embedui

// Package web serves the console.
//
// With the embedui tag the built SPA is compiled into the binary, so an
// installation is one artefact: one image to mirror into an air-gapped
// registry, and no way for a stale console pod to talk to a newer API
// (PRD DE-01). Without the tag the same handler proxies a dev server, so the
// frontend keeps hot reload during development.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// dist is populated by `make web` before the Go build. The directory is not
// committed; only its .gitignore is.
//
//go:embed all:dist
var dist embed.FS

// Handler serves the SPA with a history fallback: any path that is not a real
// asset returns index.html, because the router lives in the browser.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// Only reachable when the embed directory is missing at build time,
		// which the build itself already rejects.
		panic("web: embedded dist is unreadable: " + err.Error())
	}
	return spaHandler(http.FS(sub))
}

// Embedded reports whether this build carries the console.
func Embedded() bool { return true }
