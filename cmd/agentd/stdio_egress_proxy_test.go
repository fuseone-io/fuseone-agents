package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func TestStdioEgressProxy_forwardsOnlyConfiguredHTTPDestinations(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)
	host, port := hostPortOf(t, upstream.URL)
	proxyURL := startProxyFor(t, domain.MCPEgressDestination{Host: host, Port: port})
	client := proxiedClient(t, proxyURL)

	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("allowed response = %s %q, want 200 ok", resp.Status, body)
	}

	blocked, err := client.Get("http://" + net.JoinHostPort(host, "1") + "/")
	if err != nil {
		t.Fatalf("blocked GET: %v", err)
	}
	_ = blocked.Body.Close()
	if blocked.StatusCode != http.StatusForbidden {
		t.Fatalf("blocked status = %s, want 403", blocked.Status)
	}
}

func TestStdioEgressProxy_refusesRequestsWithoutTheLocalToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unauthenticated proxy request reached upstream")
	}))
	t.Cleanup(upstream.Close)
	host, port := hostPortOf(t, upstream.URL)
	proxyURL := startProxyFor(t, domain.MCPEgressDestination{Host: host, Port: port})
	withoutToken := stripProxyUser(t, proxyURL)
	client := proxiedClient(t, withoutToken)

	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("GET through unauthenticated proxy: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("status = %s, want 407", resp.Status)
	}
}

func TestStdioEgressProxy_recordsDenialsWithoutURLParts(t *testing.T) {
	observer := &recordingStdioEgress{}
	proxyURL := startProxyForObserver(t,
		domain.MCPEgressDestination{Host: "allowed.internal", Port: 80}, observer)
	client := proxiedClient(t, proxyURL)

	resp, err := client.Get("http://blocked.internal:8080/private?token=secret")
	if err != nil {
		t.Fatalf("blocked GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %s, want 403", resp.Status)
	}

	if len(observer.denials) != 1 {
		t.Fatalf("denials = %+v, want one", observer.denials)
	}
	got := observer.denials[0]
	if got.server != "crm" || got.host != "" || got.port != 0 {
		t.Fatalf("denial target = %+v, want server only for untrusted target", got)
	}
	if got.code != "stdio_egress_destination_denied" {
		t.Fatalf("code = %q, want destination denied", got.code)
	}
	for _, forbidden := range []string{"private", "token", "secret"} {
		if strings.Contains(got.host, forbidden) || strings.Contains(got.code, forbidden) {
			t.Fatalf("denial leaked URL part %q: %+v", forbidden, got)
		}
	}
}

func TestStdioEgressProxy_recordsConfiguredDestinationWhenItCannotConnect(t *testing.T) {
	observer := &recordingStdioEgress{}
	proxyURL := startProxyForObserver(t,
		domain.MCPEgressDestination{Host: "127.0.0.1", Port: 1}, observer)
	client := proxiedClient(t, proxyURL)

	resp, err := client.Get("http://127.0.0.1:1/")
	if err != nil {
		t.Fatalf("unavailable GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %s, want 502", resp.Status)
	}

	if len(observer.denials) != 1 {
		t.Fatalf("denials = %+v, want one", observer.denials)
	}
	got := observer.denials[0]
	if got.host != "127.0.0.1" || got.port != 1 {
		t.Fatalf("denial target = %+v, want configured host:port", got)
	}
	if got.code != "stdio_egress_destination_unavailable" {
		t.Fatalf("code = %q, want destination unavailable", got.code)
	}
}

func TestStdioEgressProxy_removesHopByHopHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{
			"Connection",
			"X-Hop",
			"TE",
			"Upgrade",
			"Proxy-Authorization",
			"Proxy-Connection",
		} {
			if got := r.Header.Get(header); got != "" {
				t.Errorf("upstream received %s=%q", header, got)
			}
		}
		w.Header().Set("Connection", "X-Upstream")
		w.Header().Set("X-Upstream", "leak")
		w.Header().Set("Keep-Alive", "timeout=5")
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(upstream.Close)
	host, port := hostPortOf(t, upstream.URL)
	proxyURL := startProxyFor(t, domain.MCPEgressDestination{Host: host, Port: port})
	client := proxiedClient(t, proxyURL)

	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Connection", "X-Hop, Upgrade")
	req.Header.Set("X-Hop", "leak")
	req.Header.Set("TE", "trailers")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Proxy-Connection", "keep-alive")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	_ = resp.Body.Close()
	for _, header := range []string{"Connection", "X-Upstream", "Keep-Alive"} {
		if got := resp.Header.Get(header); got != "" {
			t.Fatalf("client received %s=%q", header, got)
		}
	}
}

func TestStdioEgressProxy_tunnelsConnectOnlyToConfiguredDestinations(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go echoOnce(ln)
	host, port := splitAddr(t, ln.Addr().String())
	proxyURL := startProxyFor(t, domain.MCPEgressDestination{Host: host, Port: port})

	conn := connectThroughProxy(t, proxyURL, ln.Addr().String())
	defer conn.Close()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write tunnel: %v", err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read tunnel: %v", err)
	}
	if string(got) != "pong" {
		t.Fatalf("tunnel response = %q, want pong", got)
	}
}

func TestStdioEgressProxy_refusesCloudMetadataLiterals(t *testing.T) {
	_, cleanup, err := startStdioEgressProxy("crm", []domain.MCPEgressDestination{
		{Host: "169.254.169.254", Port: 80},
	}, nil)
	t.Cleanup(cleanup)
	if !errors.Is(err, errStdioEgressProxyBad) {
		t.Fatalf("startStdioEgressProxy = %v, want bad proxy policy", err)
	}
}

func TestStdioProxyPolicy_aWildcardDoesNotMatchTheApexWithALeadingDot(t *testing.T) {
	host := normalizeProxyHost(".sales.internal")
	if hostMatches("*.sales.internal", host) {
		t.Fatalf("wildcard matched %q through a leading-dot host", host)
	}
}

func startProxyFor(t *testing.T, dest domain.MCPEgressDestination) string {
	t.Helper()
	return startProxyForObserver(t, dest, nil)
}

func startProxyForObserver(
	t *testing.T, dest domain.MCPEgressDestination, observer stdioEgressObserver,
) string {
	t.Helper()
	proxy, cleanup, err := startStdioEgressProxy("crm", []domain.MCPEgressDestination{dest}, observer)
	if err != nil {
		t.Fatalf("startStdioEgressProxy: %v", err)
	}
	t.Cleanup(cleanup)
	return proxy
}

type recordingStdioEgress struct {
	denials []recordedStdioEgress
}

type recordedStdioEgress struct {
	server string
	host   string
	port   int
	code   string
}

func (r *recordingStdioEgress) StdioEgressDenied(
	_ context.Context, server, host string, port int, code string,
) {
	r.denials = append(r.denials, recordedStdioEgress{
		server: server, host: host, port: port, code: code,
	})
}

func proxiedClient(t *testing.T, proxy string) *http.Client {
	t.Helper()
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		t.Fatalf("proxy URL: %v", err)
	}
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   2 * time.Second,
	}
}

func connectThroughProxy(t *testing.T, proxyURL, target string) net.Conn {
	t.Helper()
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatalf("proxy URL: %v", err)
	}
	conn, err := net.DialTimeout("tcp", proxy.Host, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", target, target, proxyAuthHeader(proxy))
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		t.Fatalf("write CONNECT: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		t.Fatalf("CONNECT status = %s, want 200", resp.Status)
	}
	return conn
}

func proxyAuthHeader(proxy *url.URL) string {
	if proxy.User == nil {
		return ""
	}
	password, _ := proxy.User.Password()
	token := base64.StdEncoding.EncodeToString([]byte(proxy.User.Username() + ":" + password))
	return "Proxy-Authorization: Basic " + token + "\r\n"
}

func stripProxyUser(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("proxy URL: %v", err)
	}
	u.User = nil
	return u.String()
}

func echoOnce(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err == nil && string(buf) == "ping" {
		_, _ = conn.Write([]byte("pong"))
	}
}

func hostPortOf(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return splitAddr(t, u.Host)
}

func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("port %q: %v", portText, err)
	}
	return strings.Trim(host, "[]"), port
}
