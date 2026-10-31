package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/factory/guard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServeHTTP_noBearerConfigured_refusesToStart(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "factory", Version: "test"}, nil)
	// Bounded, so a server that starts anyway ends the test with a nil error
	// instead of hanging it.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	err := serveHTTP(ctx, server, "127.0.0.1:0", "")
	if !errors.Is(err, guard.ErrConfig) {
		t.Fatalf("err = %v, want a refusal to serve without a token", err)
	}
}

func TestRequireBearer_wrongOrMissingToken_unauthorised(t *testing.T) {
	t.Parallel()

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := requireBearer("s3cret", ok)
	for header, want := range map[string]int{
		"":              http.StatusUnauthorized,
		"Bearer wrong":  http.StatusUnauthorized,
		"s3cret":        http.StatusUnauthorized,
		"Bearer s3cret": http.StatusNoContent,
	} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Authorization %q: status %d, want %d", header, rec.Code, want)
		}
	}
}
