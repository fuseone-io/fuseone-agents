package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// revisionFormat is part of every revision, so changing what goes into one retires the
// old ones visibly: a client holding an old tag gets a 412 and reads again, and never a
// match that means something else.
const revisionFormat = "v1"

// Revision names one state of a resource a client manages, as a strong HTTP entity-tag
// with its quotes. It is opaque to a client and only ever compared.
type Revision string

/*
NewRevision derives a resource's revision from what it is, not from when it was written.

So a write that changes nothing leaves it alone, and who wrote last is not in it: a client
comparing revisions is asking whether the resource drifted. Secrets come as fingerprints,
which are keyed, so a tag says nothing about a secret beyond whether it changed. Their key
comes from the master key, so changing that key changes every revision of a resource that
holds a secret, and clients read again.

Each part is length-prefixed: without that, two different pairs of parts could run
together into the same bytes and name the same revision.
*/
func NewRevision(kind string, config []byte, fingerprints ...string) Revision {
	sum := sha256.New()
	part := func(b []byte) {
		sum.Write(binary.BigEndian.AppendUint64(nil, uint64(len(b))))
		sum.Write(b)
	}
	part([]byte(kind))
	part(config)
	for _, fingerprint := range fingerprints {
		part([]byte(fingerprint))
	}
	return Revision(`"` + revisionFormat + "-" + hex.EncodeToString(sum.Sum(nil)) + `"`)
}

// ErrBadIfMatch is an If-Match header that does not parse. Refused rather than ignored,
// because ignoring it would turn off the protection the client believes it asked for.
var ErrBadIfMatch = errors.New("If-Match is neither * nor a list of entity-tags")

// StaleError is a write refused by its If-Match. Current is what the resource is now,
// and empty when it does not exist.
type StaleError struct{ Current Revision }

func (e *StaleError) Error() string {
	if e.Current == "" {
		return "the resource does not exist"
	}
	return "the resource is now at " + string(e.Current)
}

// IfMatch is a parsed If-Match header. The zero value is no header, which asks nothing.
type IfMatch struct {
	present bool
	any     bool
	strong  []Revision
}

// ParseIfMatch reads the header as RFC 9110 §13.1.1 defines it. Nil is no header. A weak
// tag parses and never matches: If-Match compares strongly.
func ParseIfMatch(header *string) (IfMatch, error) {
	if header == nil {
		return IfMatch{}, nil
	}
	value := strings.Trim(*header, " \t")
	if value == "*" {
		return IfMatch{present: true, any: true}, nil
	}
	parsed := IfMatch{present: true}
	found := false
	for rest := value; rest != ""; {
		rest = strings.TrimLeft(rest, " \t,")
		if rest == "" {
			break
		}
		tag, weak, after, err := nextEntityTag(rest)
		if err != nil {
			return IfMatch{}, err
		}
		found = true
		if !weak {
			parsed.strong = append(parsed.strong, Revision(tag))
		}
		rest, err = afterListElement(after)
		if err != nil {
			return IfMatch{}, err
		}
	}
	if !found {
		return IfMatch{}, ErrBadIfMatch
	}
	return parsed, nil
}

func nextEntityTag(s string) (tag string, weak bool, rest string, err error) {
	if strings.HasPrefix(s, `W/`) {
		weak, s = true, s[2:]
	}
	if !strings.HasPrefix(s, `"`) {
		return "", false, "", ErrBadIfMatch
	}
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			return s[:i+1], weak, s[i+1:], nil
		case c < 0x21 || c == 0x7f:
			return "", false, "", fmt.Errorf("%w: a tag holds a control character or a space", ErrBadIfMatch)
		}
	}
	return "", false, "", ErrBadIfMatch
}

// afterListElement allows only whitespace before the next comma or the end.
func afterListElement(s string) (string, error) {
	trimmed := strings.TrimLeft(s, " \t")
	if trimmed != "" && trimmed[0] != ',' {
		return "", ErrBadIfMatch
	}
	return trimmed, nil
}

// Present reports that the request carried the header at all.
func (m IfMatch) Present() bool { return m.present }

// Allows reports whether a write may go ahead against what is there now. No header
// allows anything. Any header on something that does not exist refuses, `*` included:
// each one asks about a current representation, and there is none.
func (m IfMatch) Allows(exists bool, current Revision) bool {
	switch {
	case !m.present:
		return true
	case !exists:
		return false
	case m.any:
		return true
	}
	return slices.Contains(m.strong, current)
}
