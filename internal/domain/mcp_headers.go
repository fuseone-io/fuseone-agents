package domain

import (
	"errors"
	"fmt"
	"net/textproto"
	"strings"
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

// ErrHeaderInvalid is a written header HTTP cannot send, or one the transport
// owns. It names the header and never its value, which is the credential.
var ErrHeaderInvalid = errors.New("mcp headers: invalid header")

/*
transportHeaders are the headers the MCP transport sets itself. One written as
a credential would replace what the connection depends on: the address, the
body's framing, the session the server resumes. Canonical, as net/http keys
them.
*/
var transportHeaders = map[string]bool{
	"Host": true, "Content-Length": true, "Content-Type": true, "Transfer-Encoding": true,
	"Connection": true, "Keep-Alive": true, "Upgrade": true, "Te": true, "Trailer": true,
	"Proxy-Connection": true, "Accept": true, "Mcp-Session-Id": true, "Mcp-Protocol-Version": true,
}

/*
CheckWrittenHeaders judges the headers a write gives a server: each name an
HTTP token the transport does not own, each value free of control characters
but tab, and no name given twice. Judged at the write, on what was sent and not
on what is stored, so a person typing a header sees the refusal, and a server
configured before this rule keeps working until somebody rewrites its headers.
*/
func CheckWrittenHeaders(headers map[string]string) error {
	if err := CheckHeaderNames(headers); err != nil {
		return err
	}
	for name, value := range headers {
		switch {
		case !isHTTPToken(name):
			return fmt.Errorf("%w: %q is not a header name HTTP can send; use letters, digits and - _ .",
				ErrHeaderInvalid, name)
		case transportHeaders[textproto.CanonicalMIMEHeaderKey(name)]:
			return fmt.Errorf("%w: %s is set by the MCP transport and cannot carry a credential",
				ErrHeaderInvalid, textproto.CanonicalMIMEHeaderKey(name))
		case strings.ContainsFunc(value, isControl):
			return fmt.Errorf("%w: the value of %s holds a line break or another control character",
				ErrHeaderInvalid, name)
		}
	}
	return nil
}

// isHTTPToken is RFC 9110's token: what a header name may be.
func isHTTPToken(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > 0x7e || r <= ' ' || strings.ContainsRune(`"(),/:;<=>?@[\]{}`, r) {
			return false
		}
	}
	return true
}

func isControl(r rune) bool { return (r < ' ' && r != '\t') || r == 0x7f }
