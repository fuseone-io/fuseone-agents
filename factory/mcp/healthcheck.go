package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fuseone/agents/factory/guard"
)

// healthcheck is `factory-mcp healthcheck`: an exec probe for a sidecar bound
// to the pod's loopback. The kubelet sends an httpGet probe to the pod IP, or
// with `host: 127.0.0.1` to the node's own loopback, so it can never reach a
// listener on the pod's 127.0.0.1; the image has no shell and no curl to exec
// instead. Run inside the container, this shares the pod's loopback.
//
// It reads FACTORY_MCP_ADDR, the same variable the server binds, so the probe
// cannot drift from the listener. Off loopback the server speaks TLS, and the
// kubelet's own HTTPS probe reaches it: refused here rather than guessed at.
func healthcheck(ctx context.Context, getenv func(string) string, client *http.Client) error {
	addr := getenv("FACTORY_MCP_ADDR")
	if !loopbackOnly(addr) {
		return fmt.Errorf("%w: healthcheck probes a loopback FACTORY_MCP_ADDR, got %q: "+
			"off loopback the server speaks TLS, so probe it with an httpGet probe, scheme HTTPS", guard.ErrConfig, addr)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		return fmt.Errorf("healthcheck: /healthz answered %d", resp.StatusCode)
	}
	return nil
}
