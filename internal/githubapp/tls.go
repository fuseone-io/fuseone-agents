package githubapp

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

// ErrPlainHTTP is a GitHub App request addressed over plain HTTP to another
// host. It names no URL: the caller knows which one it passed.
var ErrPlainHTTP = errors.New(
	"github app: the URL must be https — every request carries the App's JWT or installation token; " +
		"plain http is accepted only to a loopback host")

/*
RequireTLS refuses a URL a GitHub App secret must not travel to.

Both requests an App makes carry a bearer: the JWT that mints, and the
installation token every MCP call sends. Over plain HTTP either one is readable
by anything on the path, and the token acts as the App for its hour. Loopback is
the one exception, because nothing leaves the machine — that is a sidecar, or a
test.
*/
func RequireTLS(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ErrPlainHTTP
	}
	switch {
	case u.Scheme == "https" && u.Hostname() != "":
		return nil
	case u.Scheme == "http" && isLoopback(u.Hostname()):
		return nil
	}
	return ErrPlainHTTP
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Unmap().IsLoopback()
}
