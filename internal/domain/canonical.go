package domain

import (
	"bytes"
	"encoding/json"
)

// CanonicalJSON returns a byte-stable encoding of a JSON document: object keys
// sorted, insignificant whitespace removed, numbers kept verbatim.
//
// The hash chain covers the payload, and a store is free to reshape JSON on
// the way in. Postgres jsonb reorders keys and strips whitespace, so hashing
// the caller's raw bytes would make every step fail verification the moment it
// came back from the database. Canonicalising on the way in and on the way out
// means the digest describes the document, not one serialisation of it.
//
// Numbers are preserved as written rather than round-tripped through float64,
// which would silently lose precision on integers above 2^53.
func CanonicalJSON(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		// Not JSON. Hash it as it stands: the caller owns the format, and
		// silently discarding a payload would be worse than a stable digest
		// over opaque bytes.
		return b
	}

	out, err := json.Marshal(v)
	if err != nil {
		return b
	}
	if bytes.Equal(out, []byte("{}")) || bytes.Equal(out, []byte("null")) {
		return nil
	}
	return out
}
