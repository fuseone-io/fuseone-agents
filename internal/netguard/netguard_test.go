package netguard

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestValidateHTTPURL_allowsPrivateNetworkServers(t *testing.T) {
	t.Parallel()

	if err := ValidateHTTPURL("http://10.24.0.17:8080/mcp"); err != nil {
		t.Fatalf("ValidateHTTPURL(private) = %v, want allowed", err)
	}
}

func TestValidateHTTPURL_refusesLiteralMetadataAddresses(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://[fd00:ec2::254]/latest/meta-data",
		"http://[fe80::1]/mcp",
	} {
		if err := ValidateHTTPURL(raw); !errors.Is(err, ErrBlockedAddress) {
			t.Fatalf("ValidateHTTPURL(%q) = %v, want ErrBlockedAddress", raw, err)
		}
	}
}

func TestDialer_refusesAHostnameThatResolvesToMetadata(t *testing.T) {
	t.Parallel()

	dialer := Dialer{
		LookupIPAddr: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, nil
		},
	}
	_, err := dialer.DialContext(t.Context(), "tcp", "metadata.google.internal:80")
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("DialContext = %v, want ErrBlockedAddress", err)
	}
}

func TestGuardHTTPTransport_makesTheGuardedDialerTheOnlyNetworkPath(t *testing.T) {
	t.Parallel()

	transport := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			t.Fatal("proxy should have been removed")
			return nil, nil
		},
		DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("custom TLS dialer should have been removed")
			return nil, nil
		},
	}
	GuardHTTPTransport(transport)

	if transport.Proxy != nil {
		t.Fatal("proxy survived the guard")
	}
	if transport.DialTLSContext != nil || transport.DialTLS != nil {
		t.Fatal("TLS dialer survived the guard")
	}
	if transport.DialContext == nil {
		t.Fatal("guard did not install a dialer")
	}
}

func TestRefuseEnvironmentProxy_refusesWithoutLeakingTheProxyURL(t *testing.T) {
	old := proxyFromEnvironment
	t.Cleanup(func() { proxyFromEnvironment = old })
	proxyFromEnvironment = func(*http.Request) (*url.URL, error) {
		return url.Parse("http://user:secret@proxy.internal:3128")
	}

	rt := RefuseEnvironmentProxy(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request escaped through the base transport")
		return nil, nil
	}))
	req, err := http.NewRequest(http.MethodGet, "https://mcp.example.com/mcp", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = rt.RoundTrip(req)
	if !errors.Is(err, ErrProxyUnsupported) {
		t.Fatalf("RoundTrip = %v, want proxy refusal", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "proxy.internal") {
		t.Fatalf("proxy refusal leaked proxy configuration: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := f(r)
	if resp == nil && err == nil {
		resp = &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}
	}
	return resp, err
}
