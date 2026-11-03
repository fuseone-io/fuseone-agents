package main

import (
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/fuseone/agents/factory/guard"
)

func addrEnv(addr string) func(string) string {
	return func(k string) string {
		if k == "FACTORY_MCP_ADDR" {
			return addr
		}
		return ""
	}
}

// The exec probe's subject: a loopback listener the kubelet cannot reach,
// because its httpGet goes to the pod IP, or with host 127.0.0.1 to the
// node's own loopback. Run inside the container, the probe shares the pod's.
func TestHealthcheck_serverListeningOnLoopback_healthy(t *testing.T) {
	t.Parallel()

	addr, err := startHTTP(t, httpOptions{Addr: "127.0.0.1:0", Token: "s3cret"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := healthcheck(t.Context(), addrEnv(addr), http.DefaultClient); err != nil {
		t.Fatalf("healthcheck = %v, want healthy", err)
	}
}

func TestHealthcheck_nothingListening_unhealthy(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	if err := healthcheck(t.Context(), addrEnv(addr), http.DefaultClient); err == nil {
		t.Fatal("healthcheck = nil with nothing listening, want an error")
	}
}

// Something listening that is not this server's /healthz is not healthy.
func TestHealthcheck_listenerAnswersOtherThanOK_unhealthy(t *testing.T) {
	t.Parallel()

	other := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	srv := &http.Server{Handler: other}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	if err := healthcheck(t.Context(), addrEnv(ln.Addr().String()), http.DefaultClient); err == nil {
		t.Fatal("healthcheck = nil on a 503, want an error")
	}
}

// Off loopback the server speaks TLS, and the kubelet's own httpGet probe
// with scheme HTTPS reaches it: the exec probe refuses rather than guess.
func TestHealthcheck_addressNotLoopbackOrUnset_refusedAsConfig(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"", ":8091", "0.0.0.0:8091", "localhost:8091"} {
		if err := healthcheck(t.Context(), addrEnv(addr), http.DefaultClient); !errors.Is(err, guard.ErrConfig) {
			t.Errorf("addr %q: healthcheck = %v, want ErrConfig", addr, err)
		}
	}
}
