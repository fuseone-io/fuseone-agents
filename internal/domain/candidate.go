package domain

import "errors"

// ErrCandidateUnreadable is a seal that cannot be read, or whose bytes are not
// the ones its claim and its row describe (NT-018 phase 2). A candidate in that
// state never becomes readable again: callers fail closed, never falling back
// to another seal of the same version or to the published version.
var ErrCandidateUnreadable = errors.New("the candidate's seal is unreadable")
