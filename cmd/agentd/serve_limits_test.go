package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithBodyLimit_oversizedBodyIsRefusedMidRead(t *testing.T) {
	t.Parallel()

	var readErr error
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	})
	srv := httptest.NewServer(withBodyLimit(inner))
	t.Cleanup(srv.Close)

	// Authorization in the generated handlers runs after the decode, so the
	// limit has to live in front of every route: otherwise any caller — or
	// anyone at all on a --no-auth installation — streams gigabytes into a
	// JSON decoder before a permission is ever checked.
	big := strings.NewReader(strings.Repeat("x", maxRequestBody+1))
	resp, err := http.Post(srv.URL+"/x", "application/json", big)
	if err == nil {
		resp.Body.Close()
	}
	var maxed *http.MaxBytesError
	if !errorsAs(readErr, &maxed) {
		t.Fatalf("handler read error = %v, want the body limit to have cut it", readErr)
	}

	small := strings.NewReader(`{"ok":true}`)
	resp, err = http.Post(srv.URL+"/x", "application/json", small)
	if err != nil {
		t.Fatalf("small post: %v", err)
	}
	resp.Body.Close()
	if readErr != nil {
		t.Fatalf("small body errored: %v", readErr)
	}
}

// errorsAs avoids importing errors just for one assertion helper.
func errorsAs(err error, target any) bool {
	if err == nil {
		return false
	}
	switch t := target.(type) {
	case **http.MaxBytesError:
		for e := err; e != nil; {
			if m, ok := e.(*http.MaxBytesError); ok {
				*t = m
				return true
			}
			u, ok := e.(interface{ Unwrap() error })
			if !ok {
				return false
			}
			e = u.Unwrap()
		}
	}
	return false
}
