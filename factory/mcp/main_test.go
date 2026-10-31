package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/factory/guard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: "factory", Version: "test"}, nil)
}

// startHTTP runs serveHTTP until the test ends and returns the bound address.
// A refusal to start is returned instead, with an empty address.
func startHTTP(t *testing.T, opts httpOptions) (string, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	bound := make(chan string, 1)
	opts.onListen = func(a net.Addr) { bound <- a.String() }
	errs := make(chan error, 1)
	go func() { errs <- serveHTTP(ctx, testServer(), opts) }()
	t.Cleanup(func() {
		cancel()
		<-errs
	})
	select {
	case addr := <-bound:
		return addr, nil
	case err := <-errs:
		errs <- err // for the cleanup
		return "", err
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP neither started nor refused")
		return "", nil
	}
}

func TestServeHTTP_noBearerConfigured_refusesToStart(t *testing.T) {
	t.Parallel()

	_, err := startHTTP(t, httpOptions{Addr: "127.0.0.1:0"})
	if !errors.Is(err, guard.ErrConfig) {
		t.Fatalf("err = %v, want a refusal to serve without a token", err)
	}
}

// A bearer sent in clear to a reachable address is a bearer anyone on the
// path can replay, calling the tools directly and skipping the platform's
// approval. Plaintext is allowed only where nobody else can listen.
func TestServeHTTP_nonLoopbackWithoutTLS_refusesToStartNamingTheFix(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{":0", "0.0.0.0:0", "[::]:0", "192.0.2.10:0", "localhost:0"} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()
			_, err := startHTTP(t, httpOptions{Addr: addr, Token: "s3cret"})
			if !errors.Is(err, guard.ErrConfig) || !strings.Contains(err.Error(), "FACTORY_MCP_TLS_CERT") {
				t.Fatalf("err = %v, want ErrConfig naming FACTORY_MCP_TLS_CERT", err)
			}
		})
	}
}

func TestServeHTTP_halfATLSConfiguration_refused(t *testing.T) {
	t.Parallel()

	pki := newPKI(t)
	_, err := startHTTP(t, httpOptions{Addr: "127.0.0.1:0", Token: "s3cret", CertFile: pki.serverCert})
	if !errors.Is(err, guard.ErrConfig) {
		t.Fatalf("err = %v, want a refusal of a certificate without a key", err)
	}
}

func TestServeHTTP_loopbackWithoutTLS_servesPlainHTTP(t *testing.T) {
	t.Parallel()

	addr, err := startHTTP(t, httpOptions{Addr: "127.0.0.1:0", Token: "s3cret"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	resp, err := http.Post("http://"+addr+"/", "application/json", nil)
	if err != nil {
		t.Fatalf("plain request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want the bearer check answering", resp.StatusCode)
	}
}

func TestServeHTTP_withTLS_servesHTTPSOnAnyAddressAndRefusesPlainHTTP(t *testing.T) {
	t.Parallel()

	pki := newPKI(t)
	addr, err := startHTTP(t, httpOptions{
		Addr: ":0", Token: "s3cret", CertFile: pki.serverCert, KeyFile: pki.serverKey,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	port := addr[strings.LastIndex(addr, ":")+1:]

	resp, err := pki.client(nil).Post("https://127.0.0.1:"+port+"/", "application/json", nil)
	if err != nil {
		t.Fatalf("https request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("https status %d, want the bearer check answering", resp.StatusCode)
	}

	plain, err := http.Post("http://127.0.0.1:"+port+"/", "application/json", nil)
	if err == nil {
		_ = plain.Body.Close()
		if plain.StatusCode != http.StatusBadRequest {
			t.Fatalf("plain HTTP reached the handler: status %d", plain.StatusCode)
		}
	}
}

func TestServeHTTP_withClientCA_requiresAClientCertificate(t *testing.T) {
	t.Parallel()

	pki := newPKI(t)
	addr, err := startHTTP(t, httpOptions{
		Addr: "127.0.0.1:0", Token: "s3cret",
		CertFile: pki.serverCert, KeyFile: pki.serverKey, ClientCAFile: pki.caFile,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if resp, err := pki.client(nil).Post("https://"+addr+"/", "application/json", nil); err == nil {
		_ = resp.Body.Close()
		t.Fatalf("a client without a certificate got status %d", resp.StatusCode)
	}
	resp, err := pki.client(&pki.clientPair).Post("https://"+addr+"/", "application/json", nil)
	if err != nil {
		t.Fatalf("mtls request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mtls status %d, want the bearer check answering", resp.StatusCode)
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

// pki is a throwaway CA with a server certificate for 127.0.0.1 and a client
// certificate, generated per test. Nothing is committed.
type pki struct {
	caFile, serverCert, serverKey string
	pool                          *x509.CertPool
	clientPair                    tls.Certificate
}

func newPKI(t *testing.T) *pki {
	t.Helper()
	dir := t.TempDir()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	p := &pki{pool: x509.NewCertPool()}
	p.pool.AddCert(ca)
	p.caFile = writePEM(t, dir, "ca.pem", "CERTIFICATE", caDER)

	leaf := func(serial int64, usage x509.ExtKeyUsage) ([]byte, *ecdsa.PrivateKey) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "leaf"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return der, key
	}
	serverDER, serverKey := leaf(2, x509.ExtKeyUsageServerAuth)
	p.serverCert = writePEM(t, dir, "server.pem", "CERTIFICATE", serverDER)
	keyDER, _ := x509.MarshalECPrivateKey(serverKey)
	p.serverKey = writePEM(t, dir, "server-key.pem", "EC PRIVATE KEY", keyDER)

	clientDER, clientKey := leaf(3, x509.ExtKeyUsageClientAuth)
	p.clientPair = tls.Certificate{Certificate: [][]byte{clientDER}, PrivateKey: clientKey}
	return p
}

func writePEM(t *testing.T, dir, name, kind string, der []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (p *pki) client(cert *tls.Certificate) *http.Client {
	cfg := &tls.Config{RootCAs: p.pool, MinVersion: tls.VersionTLS12}
	if cert != nil {
		cfg.Certificates = []tls.Certificate{*cert}
	}
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: cfg}}
}
