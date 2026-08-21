package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var (
	ErrBadURL         = errors.New("netguard: URL must be http or https with a host")
	ErrBlockedAddress = errors.New(
		"netguard: address targets cloud metadata or a link-local network")
	ErrProxyUnsupported = errors.New(
		"netguard: MCP HTTP does not support environment proxies because address validation requires local DNS resolution")
)

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fd00:ec2::/64"),
}

// ValidateHTTPURL checks what can be known before a network request happens.
//
// Private addresses are allowed: this process runs inside the customer's
// network, and on-premise MCP servers are a normal case. Cloud metadata and
// link-local ranges are not allowed, because they are credentials rather than
// tool servers.
func ValidateHTTPURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" {
		return ErrBadURL
	}
	return CheckHostLiteral(u.Hostname())
}

// CheckHostLiteral refuses a blocked IP written directly in the URL. Hostnames
// are checked later, in the dialer, after DNS has said where they point now.
func CheckHostLiteral(host string) error {
	addr, ok := parseAddr(host)
	if !ok {
		return nil
	}
	if IsBlocked(addr) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
	}
	return nil
}

func IsBlocked(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// GuardHTTPTransport makes a Transport refuse cloud metadata and link-local
// endpoints at the moment it dials.
//
// The proxy is deliberately disabled. Once a proxy receives a hostname, the
// proxy does the DNS resolution and this worker can no longer prove the request
// will not be sent to a metadata address after a DNS rebind.
func GuardHTTPTransport(t *http.Transport) {
	t.Proxy = nil
	t.DialTLS = nil
	t.DialTLSContext = nil
	t.DialContext = Dialer{}.DialContext
}

var proxyFromEnvironment = http.ProxyFromEnvironment

func RefuseEnvironmentProxy(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return proxyRefusal{base: base}
}

type proxyRefusal struct {
	base http.RoundTripper
}

func (p proxyRefusal) RoundTrip(r *http.Request) (*http.Response, error) {
	proxy, err := proxyFromEnvironment(r)
	if err != nil {
		return nil, fmt.Errorf("netguard: read proxy configuration: %w", err)
	}
	if proxy != nil {
		return nil, ErrProxyUnsupported
	}
	return p.base.RoundTrip(r)
}

type Dialer struct {
	Dialer       *net.Dialer
	LookupIPAddr func(context.Context, string) ([]net.IPAddr, error)
}

func (d Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	dialer := d.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	}
	if addr, ok := parseAddr(host); ok {
		if IsBlocked(addr) {
			return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
	}

	lookup := d.LookupIPAddr
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("netguard: no addresses for %s", host)
	}

	blocked := 0
	var lastErr error
	for _, resolved := range addrs {
		addr, ok := addrFromIP(resolved.IP)
		if !ok {
			continue
		}
		if IsBlocked(addr) {
			blocked++
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if blocked == len(addrs) {
		return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, host)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("netguard: no usable addresses for %s", host)
}

func parseAddr(host string) (netip.Addr, bool) {
	host = strings.Trim(host, "[]")
	if zone, _, found := strings.Cut(host, "%"); found {
		host = zone
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func addrFromIP(ip net.IP) (netip.Addr, bool) {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}
