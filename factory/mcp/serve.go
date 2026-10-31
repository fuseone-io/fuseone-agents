package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/fuseone/agents/factory/guard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// httpOptions configure the streamable HTTP transport.
type httpOptions struct {
	Addr, Token string
	// CertFile and KeyFile turn on TLS. Without them the server binds only to
	// a loopback address, for a sidecar behind a TLS-terminating proxy.
	CertFile, KeyFile string
	// ClientCAFile, when set, requires every client to present a certificate
	// signed by it (mTLS). Optional, because FuseOne's MCP client sends a
	// bearer, not a client certificate; use it when a proxy in front can.
	ClientCAFile string
	onListen     func(net.Addr) // tests learn the bound port
}

// serveHTTP serves the tools over HTTP behind a bearer token. Over a network
// the bearer must travel inside TLS: anyone who reads it can call the tools
// directly, skipping the platform's approval. So plaintext is refused on any
// address but loopback, and the server refuses to start rather than warn.
func serveHTTP(ctx context.Context, server *mcp.Server, opts httpOptions) error {
	ln, err := listen(opts)
	if err != nil {
		return err
	}
	if opts.onListen != nil {
		opts.onListen(ln.Addr())
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	srv := &http.Server{Handler: requireBearer(opts.Token, handler), ReadHeaderTimeout: 10 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- srv.Serve(ln) }()
	fmt.Fprintf(os.Stderr, "factory-mcp listening on %s\n", ln.Addr())

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		if err := <-errs; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// listen validates the transport and opens the listener, TLS-wrapped when
// configured.
func listen(opts httpOptions) (net.Listener, error) {
	if opts.Token == "" {
		return nil, fmt.Errorf("%w: FACTORY_MCP_TOKEN is required when FACTORY_MCP_ADDR is set", guard.ErrConfig)
	}
	cfg, err := opts.tlsConfig()
	if err != nil {
		return nil, err
	}
	if cfg == nil && !loopbackOnly(opts.Addr) {
		return nil, fmt.Errorf("%w: FACTORY_MCP_ADDR %q is not a loopback address, and the bearer would cross "+
			"the network in clear: set FACTORY_MCP_TLS_CERT and FACTORY_MCP_TLS_KEY, or bind 127.0.0.1 or "+
			"[::1] behind a TLS-terminating proxy", guard.ErrConfig, opts.Addr)
	}
	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", opts.Addr, err)
	}
	if cfg != nil {
		ln = tls.NewListener(ln, cfg)
	}
	return ln, nil
}

// tlsConfig returns nil when TLS is not configured, and refuses a partial
// configuration rather than quietly serving plaintext.
func (o httpOptions) tlsConfig() (*tls.Config, error) {
	if o.CertFile == "" && o.KeyFile == "" && o.ClientCAFile == "" {
		return nil, nil
	}
	if o.CertFile == "" || o.KeyFile == "" {
		return nil, fmt.Errorf("%w: FACTORY_MCP_TLS_CERT and FACTORY_MCP_TLS_KEY go together "+
			"(and FACTORY_MCP_CLIENT_CA needs both)", guard.ErrConfig)
	}
	pair, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("%w: TLS certificate: %w", guard.ErrConfig, err)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	if o.ClientCAFile == "" {
		return cfg, nil
	}
	raw, err := os.ReadFile(o.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("%w: FACTORY_MCP_CLIENT_CA: %w", guard.ErrConfig, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("%w: FACTORY_MCP_CLIENT_CA holds no PEM certificate", guard.ErrConfig)
	}
	cfg.ClientCAs, cfg.ClientAuth = pool, tls.RequireAndVerifyClientCert
	return cfg, nil
}

// loopbackOnly accepts a literal loopback IP. A hostname, even localhost, is
// refused: what it resolves to is not this process's decision.
func loopbackOnly(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requireBearer(want string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, []byte("Bearer "+want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
