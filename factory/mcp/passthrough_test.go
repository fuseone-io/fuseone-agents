package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fuseone/agents/factory/guard"
)

// passThroughEnv is a complete pass-through configuration for the board
// roles, with a shared token left over from an own-App setup.
func passThroughEnv() map[string]string {
	return map[string]string{
		"FACTORY_MCP_AUTH":          "github-app-token",
		"FACTORY_ROLE_BOTS":         "triage=101,developer=103",
		"FACTORY_PROJECT_ID":        "PVT_testproject",
		"FACTORY_STATUS_FIELD_ID":   "PVTSSF_status",
		"FACTORY_LANES":             "Triagem=a,Preparado=b,Doing=c,Travado=d,Done=e",
		"FACTORY_WORKTYPE_FIELD_ID": "PVTSSF_worktype",
		"FACTORY_WORKTYPES":         "bug=f,feature=g,chore=h,docs=i",
		"FACTORY_SIZE_FIELD_ID":     "PVTSSF_size",
		"FACTORY_SIZES":             "S=j,M=k,L=l",
		"FACTORY_MCP_TOKEN":         "s3cret",
	}
}

func loadEnv(t *testing.T, env map[string]string) (guard.Config, func(string) string) {
	t.Helper()
	getenv := func(k string) string { return env[k] }
	cfg, err := guard.LoadConfig(getenv, func(string) ([]byte, error) { return nil, errors.New("no files") })
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg, getenv
}

// In pass-through mode the bearer is the caller's GitHub token, so the
// shared FACTORY_MCP_TOKEN is never read, even when it is set.
func TestHTTPOptions_passThrough_ignoresTheSharedToken(t *testing.T) {
	t.Parallel()

	cfg, getenv := loadEnv(t, passThroughEnv())
	if opts := httpOptionsFromEnv(cfg, getenv); !opts.PassThrough || opts.Token != "" {
		t.Fatalf("opts = %+v, want pass-through with no shared token", opts)
	}
}

// The shared bearer check is not in front of a pass-through handler: a
// request bearing a GitHub token, not FACTORY_MCP_TOKEN, reaches it intact.
func TestServeHTTP_passThrough_bearerReachesTheHandlerIntact(t *testing.T) {
	t.Parallel()

	seen := make(chan string, 1)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	})
	addr, err := startHandler(t, inner, httpOptions{Addr: "127.0.0.1:0", PassThrough: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/", nil)
	req.Header.Set("Authorization", "Bearer ghs_installation")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || <-seen != "Bearer ghs_installation" {
		t.Fatalf("status %d, want the handler reached with the caller's bearer", resp.StatusCode)
	}
}

// A GitHub token in clear across a network is worse than a shared secret:
// the loopback-or-TLS rule holds in pass-through mode too.
func TestServeHTTP_passThroughNonLoopbackWithoutTLS_refusesToStart(t *testing.T) {
	t.Parallel()

	_, err := startHandler(t, testHandler(), httpOptions{Addr: "0.0.0.0:0", PassThrough: true})
	if !errors.Is(err, guard.ErrConfig) || !strings.Contains(err.Error(), "FACTORY_MCP_TLS_CERT") {
		t.Fatalf("err = %v, want ErrConfig naming FACTORY_MCP_TLS_CERT", err)
	}
}

func TestRun_passThroughOverStdio_refused(t *testing.T) {
	t.Parallel()

	env := passThroughEnv()
	err := run(t.Context(), func(k string) string { return env[k] }, func(string) ([]byte, error) { return nil, nil })
	if !errors.Is(err, guard.ErrConfig) || !strings.Contains(err.Error(), "FACTORY_MCP_ADDR") {
		t.Fatalf("err = %v, want ErrConfig naming FACTORY_MCP_ADDR", err)
	}
}

// The real pass-through handler behind serveHTTP: no bearer, no tools.
func TestServeHTTP_passThroughWithoutBearer_unauthorised(t *testing.T) {
	t.Parallel()

	cfg, _ := loadEnv(t, passThroughEnv())
	handler, err := guard.NewPassThroughHandler(cfg, newClient(cfg, guard.RequestTokenSource{}), guard.SystemClock{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	addr, err := startHandler(t, handler, httpOptions{Addr: "127.0.0.1:0", PassThrough: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp, err := http.Post("http://"+addr+"/", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

// Readiness needs no credential and returns none.
func TestServeHTTP_healthz_answersWithoutAuthAndCarriesNoSecret(t *testing.T) {
	t.Parallel()

	addr, err := startHTTP(t, httpOptions{Addr: "127.0.0.1:0", Token: "s3cret"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("status %d, body %q", resp.StatusCode, body)
	}
}
