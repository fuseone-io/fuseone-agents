package web

import (
	"net/http"
	"strconv"
	"strings"
)

// variant is a precompressed copy the build writes beside a file.
type variant struct {
	coding string // the Content-Encoding token
	suffix string // the extension the build appends
}

// variants in the server's order of preference: brotli is smaller than gzip
// for the same JavaScript, so a client that takes both gets brotli.
var variants = [...]variant{
	{coding: "br", suffix: ".br"},
	{coding: "gzip", suffix: ".gz"},
}

// openEncoded opens the best precompressed variant of name that the client
// accepts, or the plain file when none fits. The returned coding is empty for
// the plain file.
func openEncoded(fsys http.FileSystem, name, acceptEncoding string) (http.File, string, error) {
	for _, v := range variants {
		if !accepts(acceptEncoding, v.coding) {
			continue
		}
		if f, ok := openRegular(fsys, name+v.suffix); ok {
			return f, v.coding, nil
		}
	}
	f, err := fsys.Open(name)
	return f, "", err
}

// openRegular opens name only if it is a regular file; a directory or a miss
// both answer false, with nothing left open.
func openRegular(fsys http.FileSystem, name string) (http.File, bool) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, false
	}
	if info, statErr := f.Stat(); statErr != nil || info.IsDir() {
		_ = f.Close()
		return nil, false
	}
	return f, true
}

// accepts reports whether an Accept-Encoding header admits coding. Only an
// explicit mention counts — a bare "*" is not taken as licence to send
// brotli — and a mention with q=0 is a refusal (RFC 9110 §12.5.3).
func accepts(header, coding string) bool {
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(part, ";")
		if strings.EqualFold(strings.TrimSpace(token), coding) {
			return qualityAboveZero(params)
		}
	}
	return false
}

// qualityAboveZero reads the q parameter of one Accept-Encoding member. An
// absent q means 1; an unreadable one is treated as a refusal, since sending
// an encoding the client may not decode is worse than sending plain bytes.
func qualityAboveZero(params string) bool {
	for _, p := range strings.Split(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return err == nil && q > 0
	}
	return true
}
