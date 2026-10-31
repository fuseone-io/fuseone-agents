package domain

import (
	"errors"
	"net/textproto"
)

// ErrHeaderCollision is a header map that names one header twice. It names no
// header and no value; the caller knows what it sent.
var ErrHeaderCollision = errors.New(
	"mcp headers: one header name is given twice in different letter case — HTTP names are " +
		"case-insensitive, so give each header once")

/*
CheckHeaderNames refuses a header map that names one header twice.

HTTP header names are case-insensitive: "X-MCP-Readonly" and "x-mcp-readonly"
are one header. A map can still hold both, and a transport that sets them in
map order sends a different value on each request — a toolset that flaps
widens discovery, and the catalogue never forgets what it saw. Canonicalised
the way net/http compares names; net/textproto is pure, no I/O.
*/
func CheckHeaderNames(headers map[string]string) error {
	seen := make(map[string]bool, len(headers))
	for name := range headers {
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if seen[canonical] {
			return ErrHeaderCollision
		}
		seen[canonical] = true
	}
	return nil
}
