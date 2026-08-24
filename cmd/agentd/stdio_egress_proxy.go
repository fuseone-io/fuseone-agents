package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/egressmetrics"
	"github.com/fuseone/agents/internal/netguard"
)

type stdioEgressProxy struct {
	server    string
	policy    stdioProxyPolicy
	token     string
	dialer    netguard.Dialer
	transport http.RoundTripper
	observer  stdioEgressObserver
}

const stdioProxyTunnelIdleTimeout = 5 * time.Minute

type stdioEgressObserver interface {
	StdioEgressDenied(ctx context.Context, server, host string, port int, code string)
}

func startStdioEgressProxy(
	server string, destinations []domain.MCPEgressDestination, observer stdioEgressObserver,
) (string, func(), error) {
	policy, err := newStdioProxyPolicy(destinations)
	if err != nil {
		return "", noop, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", noop, fmt.Errorf("start local stdio egress proxy: %w", err)
	}
	token, err := stdioProxyToken()
	if err != nil {
		_ = ln.Close()
		return "", noop, err
	}
	proxy := &stdioEgressProxy{
		server:    server,
		policy:    policy,
		token:     token,
		dialer:    guardedProxyDialer(),
		transport: guardedProxyTransport(),
		observer:  observer,
	}
	srv := &http.Server{Handler: proxy, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan struct{})
	go serveStdioProxy(server, srv, ln, done)
	cleanup := func() { stopStdioProxy(srv, ln, done) }
	return stdioProxyURL(ln.Addr().String(), token), cleanup, nil
}

func stdioProxyToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("create stdio egress proxy token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func stdioProxyURL(addr, token string) string {
	u := url.URL{Scheme: "http", Host: addr, User: url.UserPassword("fuseone", token)}
	return u.String()
}

func serveStdioProxy(server string, srv *http.Server, ln net.Listener, done chan<- struct{}) {
	defer close(done)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Warn("stdio egress proxy stopped unexpectedly", "server", server, "err", err)
	}
}

func stopStdioProxy(srv *http.Server, ln net.Listener, done <-chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	_ = ln.Close()
	<-done
}

func guardedProxyDialer() netguard.Dialer {
	return netguard.Dialer{
		Dialer: &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second},
	}
}

func guardedProxyTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	netguard.GuardHTTPTransport(t)
	return t
}

func (p *stdioEgressProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.authorized(r) {
		p.recordDenial(r.Context(), egressmetrics.CodeUnauthorized, proxyTarget{})
		w.Header().Set("Proxy-Authenticate", `Basic realm="fuseone-stdio-egress"`)
		http.Error(w, "stdio egress proxy requires its local token", http.StatusProxyAuthRequired)
		return
	}
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	if r.URL == nil || r.URL.Scheme != "http" || r.URL.Host == "" || r.URL.User != nil {
		p.recordDenial(r.Context(), egressmetrics.CodeBadRequest, proxyTarget{})
		http.Error(w, "stdio egress proxy expects absolute http requests", http.StatusBadRequest)
		return
	}
	target, code, err := p.policy.checkedTarget(r.URL, 80)
	if err != nil {
		p.recordDenial(r.Context(), code, target)
		http.Error(w, "egress destination is not allowed", http.StatusForbidden)
		return
	}
	p.forwardHTTP(w, r, target)
}

func (p *stdioEgressProxy) authorized(r *http.Request) bool {
	header := r.Header.Get("Proxy-Authorization")
	scheme, value, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	want := []byte("fuseone:" + p.token)
	return subtle.ConstantTimeCompare(decoded, want) == 1
}

func (p *stdioEgressProxy) forwardHTTP(w http.ResponseWriter, r *http.Request, target proxyTarget) {
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.Header = r.Header.Clone()
	removeHopByHopHeaders(out.Header)
	resp, err := p.transport.RoundTrip(out)
	if err != nil {
		p.recordDenial(r.Context(), codeForProxyDialError(err), target)
		http.Error(w, "egress destination is unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	headers := resp.Header.Clone()
	removeHopByHopHeaders(headers)
	copyHeader(w.Header(), headers)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *stdioEgressProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	target, code, err := p.policy.checkedAuthority(r.Host, 0)
	if err != nil {
		p.recordDenial(r.Context(), code, target)
		http.Error(w, "egress destination is not allowed", http.StatusForbidden)
		return
	}
	upstream, err := p.dialer.DialContext(r.Context(), "tcp", target.address())
	if err != nil {
		p.recordDenial(r.Context(), codeForProxyDialError(err), target)
		http.Error(w, "egress destination is unavailable", http.StatusBadGateway)
		return
	}
	p.hijackConnect(w, upstream)
}

func (p *stdioEgressProxy) recordDenial(ctx context.Context, code string, target proxyTarget) {
	if p.observer == nil {
		return
	}
	host, port := "", 0
	if target.recordable() {
		host, port = target.host, target.port
	}
	p.observer.StdioEgressDenied(ctx, p.server, host, port, code)
}

func codeForProxyDialError(err error) string {
	if errors.Is(err, netguard.ErrBlockedAddress) {
		return egressmetrics.CodeMetadataRefused
	}
	return egressmetrics.CodeDestinationUnavailable
}

func (p *stdioEgressProxy) hijackConnect(w http.ResponseWriter, upstream net.Conn) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(w, "stdio egress proxy cannot tunnel this response", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	tunnelConns(client, upstream, buffered.Reader)
}

func tunnelConns(client, upstream net.Conn, buffered *bufio.Reader) {
	var wg sync.WaitGroup
	wg.Add(2)
	clientIdle := idleConn{Conn: client, idle: stdioProxyTunnelIdleTimeout}
	upstreamIdle := idleConn{Conn: upstream, idle: stdioProxyTunnelIdleTimeout}
	go proxyCopy(&wg, upstreamIdle, io.MultiReader(buffered, clientIdle), client, upstream)
	go proxyCopy(&wg, clientIdle, upstreamIdle, upstream, client)
	wg.Wait()
}

type idleConn struct {
	net.Conn
	idle time.Duration
}

func (c idleConn) Read(p []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.idle))
	return c.Conn.Read(p)
}

func (c idleConn) Write(p []byte) (int, error) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(c.idle))
	return c.Conn.Write(p)
}

func proxyCopy(wg *sync.WaitGroup, dst io.Writer, src io.Reader, closers ...io.Closer) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	for _, closer := range closers {
		_ = closer.Close()
	}
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func removeHopByHopHeaders(h http.Header) {
	for _, field := range connectionFields(h.Get("Connection")) {
		h.Del(field)
	}
	for _, field := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Proxy-Connection",
		"TE",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		h.Del(field)
	}
}

func connectionFields(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if field := strings.TrimSpace(part); field != "" {
			out = append(out, field)
		}
	}
	return out
}

type stdioProxyPolicy struct {
	destinations []domain.MCPEgressDestination
}

func newStdioProxyPolicy(destinations []domain.MCPEgressDestination) (stdioProxyPolicy, error) {
	if len(destinations) == 0 {
		return stdioProxyPolicy{}, errStdioEgressProxyBad
	}
	out := make([]domain.MCPEgressDestination, 0, len(destinations))
	for _, dest := range destinations {
		if !validProxyDestination(dest) {
			return stdioProxyPolicy{}, errStdioEgressProxyBad
		}
		out = append(out, dest)
	}
	return stdioProxyPolicy{destinations: out}, nil
}

func validProxyDestination(dest domain.MCPEgressDestination) bool {
	if !domain.ValidMCPEgressDestination(dest) {
		return false
	}
	if strings.HasPrefix(dest.Host, "*.") {
		return true
	}
	return netguard.CheckHostLiteral(dest.Host) == nil
}

func (p stdioProxyPolicy) checkedTarget(u *url.URL, defaultPort int) (proxyTarget, string, error) {
	return p.checkedAuthority(u.Host, defaultPort)
}

func (p stdioProxyPolicy) checkedAuthority(authority string, defaultPort int) (proxyTarget, string, error) {
	target, err := parseProxyTarget(authority, defaultPort)
	if err != nil {
		return proxyTarget{}, egressmetrics.CodeBadRequest, errStdioEgressProxyBad
	}
	recordable, ok := p.allows(target)
	target.record = recordable
	if !ok {
		return target, egressmetrics.CodeDestinationDenied, errStdioEgressProxyBad
	}
	if err := netguard.CheckHostLiteral(target.host); err != nil {
		return target, egressmetrics.CodeMetadataRefused, err
	}
	return target, "", nil
}

func (p stdioProxyPolicy) allows(target proxyTarget) (bool, bool) {
	for _, dest := range p.destinations {
		if dest.Port != target.port || !hostMatches(dest.Host, target.host) {
			continue
		}
		// Only an exact configured destination is durable evidence. A denied
		// target, or a wildcard child picked by the stdio process, is untrusted
		// input and can carry unbounded cardinality or encoded secrets.
		return !strings.HasPrefix(dest.Host, "*.") && dest.Host == target.host, true
	}
	return false, false
}

func hostMatches(pattern, host string) bool {
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return host != strings.TrimPrefix(suffix, ".") && strings.HasSuffix(host, suffix)
	}
	return host == pattern
}

type proxyTarget struct {
	host   string
	port   int
	record bool
}

func (t proxyTarget) recordable() bool {
	if !t.record || t.port < 1 || t.port > 65535 || t.host == "" {
		return false
	}
	if domain.ValidMCPEgressDestination(domain.MCPEgressDestination{Host: t.host, Port: t.port}) {
		return true
	}
	if _, err := netip.ParseAddr(t.host); err == nil {
		return true
	}
	return false
}

func parseProxyTarget(authority string, defaultPort int) (proxyTarget, error) {
	host, portText, err := splitAuthority(authority, defaultPort)
	if err != nil {
		return proxyTarget{}, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return proxyTarget{}, fmt.Errorf("bad target port")
	}
	return proxyTarget{host: normalizeProxyHost(host), port: port}, nil
}

func splitAuthority(authority string, defaultPort int) (string, string, error) {
	host, port, err := net.SplitHostPort(authority)
	if err == nil {
		return host, port, nil
	}
	if defaultPort == 0 || strings.Contains(authority, ":") {
		return "", "", err
	}
	return authority, strconv.Itoa(defaultPort), nil
}

func normalizeProxyHost(host string) string {
	host = strings.Trim(host, "[]")
	host = strings.TrimSuffix(host, ".")
	host = strings.TrimPrefix(host, ".")
	return strings.ToLower(host)
}

func (t proxyTarget) address() string {
	return net.JoinHostPort(t.host, strconv.Itoa(t.port))
}
