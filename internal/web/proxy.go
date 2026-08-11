//go:build !embedui

// Package web serves the console.
//
// Without the embedui tag the binary carries no assets and forwards to a Vite
// dev server instead, so the frontend keeps hot module reload while the API
// runs as it does in production. The production build uses the tag; see
// embed.go for that half.
package web

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

// DevServerEnv names the variable that points at the running Vite server.
const DevServerEnv = "FUSEONE_WEB_DEV_URL"

// Handler proxies to the dev server named by FUSEONE_WEB_DEV_URL, defaulting
// to Vite's usual address.
func Handler() http.Handler {
	raw := os.Getenv(DevServerEnv)
	if raw == "" {
		raw = "http://127.0.0.1:5173"
	}

	target, err := url.Parse(raw)
	if err != nil {
		return explain(fmt.Sprintf("%s is not a valid URL: %v", DevServerEnv, err))
	}
	return httputil.NewSingleHostReverseProxy(target)
}

// explain answers with the fix rather than a blank 500, since the only person
// who ever sees this is a developer who has not started Vite.
func explain(reason string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "console unavailable: %s\n\n"+
			"This binary was built without the embedui tag, so it forwards to a dev server.\n"+
			"Run `npm run dev` in web/, or build with `make build` to embed the console.\n", reason)
	})
}

// Embedded reports whether this build carries the console.
func Embedded() bool { return false }
