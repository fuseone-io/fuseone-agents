package domain

import "errors"

// ErrContentErased means a reference pointed at content that was deliberately
// removed — under retention, or on a subject's request (AU-11, NF-09).
//
// Here rather than in either store because both of them raise it and a caller
// has to match one value: two sentinels for the same fact is a caller that
// handles the durable store and not the fake, and finds out in production.
//
// Distinct from a reference that resolves to nothing. One is a deletion
// somebody performed and the other is a reference that was always wrong, and
// a trail pointing at neither has to say which.
var ErrContentErased = errors.New("content was erased")

/*
ContentMetadata is what a stored payload says about itself without being read.

The reference carries only the first 16 hex of the digest, and re-hashing the
bytes read back would disagree with the record for any payload the store
truncated. So a caller checking a citation against the ledger asks for this
rather than computing it: the digest here is the one the store wrote down, over
the whole payload, at the moment it was written.

Erased is a value rather than an error because erasure and absence are different
facts and both have to be answerable. The bytes are gone; the digest is not, and
the step that referenced them still carries it, so a reader can tell a citation
whose content was erased from a citation that was never true.
*/
type ContentMetadata struct {
	Digest string
	Erased bool
}
