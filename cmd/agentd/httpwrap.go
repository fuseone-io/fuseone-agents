// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// The handler wrappers the API is served behind.

// apiProblems keeps every response under /api/ in the contract's error shape.
//
// The generated router answers an unrouted path with net/http's plain-text
// 404, which a client parsing application/problem+json cannot read. Anything
// the contract does not describe still has to look like the contract.
func apiProblems(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &problemRecorder{ResponseWriter: w, path: r.URL.Path}
		next.ServeHTTP(rec, r)
	})
}

type problemRecorder struct {
	http.ResponseWriter
	path      string
	swallowed bool
}

func (p *problemRecorder) WriteHeader(code int) {
	// net/http's NotFound sets text/plain before calling WriteHeader, so the
	// test is "did something other than the contract answer this", not
	// "is the content type unset".
	if code == http.StatusNotFound && !strings.Contains(p.Header().Get("Content-Type"), "json") {
		p.swallowed = true
		writeProblem(p.ResponseWriter, http.StatusNotFound, "Unknown endpoint",
			"No operation is defined for "+p.path)
		return
	}
	p.ResponseWriter.WriteHeader(code)
}

func (p *problemRecorder) Write(b []byte) (int, error) {
	if p.swallowed {
		// The problem body is already written; drop the router's plain text.
		return len(b), nil
	}
	return p.ResponseWriter.Write(b)
}

// maxRequestBody bounds every request body before the router sees it. The
// generated handlers decode before the route's own authorization runs, so
// the ceiling has to stand in front of all of them; the few endpoints with
// smaller ceilings of their own keep them, since the smaller bound wins.
const maxRequestBody = 2 << 20

// requestProblem says what a request did wrong with the status that names
// it: a body over the ceiling is 413, not a generic bad request — the
// difference between "shrink it" and "fix it" for whoever reads the answer.
func requestProblem(w http.ResponseWriter, _ *http.Request, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeProblem(w, http.StatusRequestEntityTooLarge, "Request body too large",
			fmt.Sprintf("the body exceeds the %d byte ceiling", tooBig.Limit))
		return
	}
	writeProblem(w, http.StatusBadRequest, "Invalid request", err.Error())
}

func withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		}
		next.ServeHTTP(w, r)
	})
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"title":%q,"status":%d,"detail":%q}`, title, status, detail)
}
