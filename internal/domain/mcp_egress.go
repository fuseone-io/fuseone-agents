package domain

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

/*
MCPEgress is where an MCP server stored outside the installation may send
requests (NT-019): a list of exact origins the platform keeps, and the area
cannot widen.

netguard already refuses cloud metadata and link-local addresses for every
server. Private addresses stay reachable on purpose, because on-premise MCP
servers are normal, so delegating servers to areas would delegate reaching any
endpoint the worker can. The list is what keeps that reach the platform's: the
server's URL, every redirect and every token endpoint must be one of its
origins. Nil is a server of the installation, which keeps the behaviour it
always had; empty is a policy that allows nothing.
*/
type MCPEgress struct {
	Origins []string
}

// ErrNotAnOrigin is a value that is not exactly a scheme, a host and a port.
var ErrNotAnOrigin = errors.New("not an origin: scheme, host and optional port only")

// CanonicalOrigin is the one form an origin is stored and compared in:
// lower-case scheme and host, and the port always written.
func CanonicalOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotAnOrigin, err)
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return "", fmt.Errorf("%w: %q", ErrNotAnOrigin, raw)
	}
	origin, ok := originOf(u)
	if !ok || strings.Contains(u.Hostname(), "*") {
		return "", fmt.Errorf("%w: %q", ErrNotAnOrigin, raw)
	}
	return origin, nil
}

// Allows says whether a request to u may be sent under this policy.
func (e *MCPEgress) Allows(u *url.URL) bool {
	if e == nil {
		return true
	}
	origin, ok := originOf(u)
	return ok && slices.Contains(e.Origins, origin)
}

// originOf is u's scheme, host and port in canonical form, or false for a URL
// that has none an HTTP client could send to.
func originOf(u *url.URL) (string, bool) {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if (scheme != "http" && scheme != "https") || host == "" {
		return "", false
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[scheme]
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", false
	}
	// Written from the number: ":0443" and ":443" are one port.
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(n)), true
}
