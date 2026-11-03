package main

import (
	"bufio"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The sidecar's whole contract with its pod, run against the built binary
// from its real entry: the environment a chart gives it, a loopback bind,
// no credential of its own, and a /healthz the kubelet can probe. Every
// other test drives serveHTTP or run directly, which cannot see a variable
// the process never reads from os.Getenv, or a main that never serves.
func TestBinary_passThroughOnLoopback_answersHealthzAndStopsOnSIGTERM(t *testing.T) {
	t.Parallel()

	bin := filepath.Join(t.TempDir(), "factory-mcp")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin)
	cmd.Env = sidecarEnv()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	addr := boundAddr(t, stderr)
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("healthz: status %d, body %q", resp.StatusCode, body)
	}

	// The kubelet stops a container with SIGTERM: a clean exit, not a kill.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit after SIGTERM: %v", err)
	}
}

// sidecarEnv is the pass-through configuration the chart's ConfigMap gives
// the sidecar, and nothing else from the test's own environment: no
// FACTORY_MCP_TOKEN and no GITHUB_APP_*, because the sidecar holds none.
func sidecarEnv() []string {
	env := []string{"FACTORY_MCP_ADDR=127.0.0.1:0"}
	for k, v := range passThroughEnv() {
		if k != "FACTORY_MCP_TOKEN" {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "FACTORY_REPOSITORY=fuseone-io/fuseone-agents")
}

// boundAddr reads the address the process announces on stderr, and fails
// the test with whatever it printed if it exits or stalls first.
func boundAddr(t *testing.T, stderr io.Reader) string {
	t.Helper()
	const prefix = "factory-mcp listening on "
	found := make(chan string, 1)
	var seen strings.Builder
	go func() {
		scan := bufio.NewScanner(stderr)
		for scan.Scan() {
			line := scan.Text()
			seen.WriteString(line + "\n")
			if strings.HasPrefix(line, prefix) {
				found <- strings.TrimPrefix(line, prefix)
			}
		}
		close(found)
	}()
	select {
	case addr, ok := <-found:
		if !ok {
			t.Fatalf("exited without listening:\n%s", seen.String())
		}
		return addr
	case <-time.After(30 * time.Second):
		t.Fatalf("did not listen within 30s")
		return ""
	}
}
