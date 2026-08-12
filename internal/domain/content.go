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
