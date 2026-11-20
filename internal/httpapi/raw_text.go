package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

// maxTypedTextBody bounds what RefuseRewrittenText reads: the largest text a guarded route
// takes, an answer for the world of 256 KiB, written entirely in six-byte \u escapes, fits
// with room to spare, and it is the server's own body ceiling, which no body passes anyway.
const maxTypedTextBody = 2 << 20

/*
RefuseRewrittenText refuses, on the routes the patterns name, a JSON body whose text the
decoder would rewrite: bytes that are not UTF-8, and a \u escape of an unpaired surrogate.
encoding/json replaces either with U+FFFD and says nothing, so the handler would keep, as
what a person typed, text they never sent. Judged on the raw bytes, before any decoder.

A pattern is a method and a path, where a star stands for one path segment, as path.Match
reads it. Every other request passes untouched.
*/
func RefuseRewrittenText(next http.Handler, patterns ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || !matchesAny(r, patterns) {
			next.ServeHTTP(w, r)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTypedTextBody))
		if err != nil {
			writeProblemJSON(w, http.StatusBadRequest, CodeInvalidInput, "Invalid request", "the body could not be read")
			return
		}
		if !utf8.Valid(raw) || hasLoneSurrogate(raw) {
			writeProblemJSON(w, http.StatusBadRequest, CodeInvalidInput, "Invalid request", "the body is not UTF-8 text")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		next.ServeHTTP(w, r)
	})
}

func matchesAny(r *http.Request, patterns []string) bool {
	for _, p := range patterns {
		method, route, ok := strings.Cut(p, " ")
		if !ok || method != r.Method {
			continue
		}
		if matched, err := path.Match(route, r.URL.Path); err == nil && matched {
			return true
		}
	}
	return false
}

/*
hasLoneSurrogate reports a \u escape of a surrogate that is not the high half of a pair
followed at once by its low half. Escapes are read in order, so an escaped backslash is
consumed whole and the "u" after it is text, not an escape.
*/
func hasLoneSurrogate(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			continue
		}
		if raw[i+1] != 'u' {
			i++
			continue
		}
		unit, ok := escapedUnit(raw, i)
		switch {
		case !ok:
			i++
		case unit >= 0xDC00 && unit <= 0xDFFF:
			return true
		case unit >= 0xD800 && unit <= 0xDBFF:
			low, paired := escapedUnit(raw, i+6)
			if !paired || low < 0xDC00 || low > 0xDFFF {
				return true
			}
			i += 11
		default:
			i += 5
		}
	}
	return false
}

// escapedUnit is the UTF-16 unit of the \uXXXX escape starting at i, if one is there.
func escapedUnit(raw []byte, i int) (uint64, bool) {
	if i+6 > len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
		return 0, false
	}
	unit, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
	return unit, err == nil
}
