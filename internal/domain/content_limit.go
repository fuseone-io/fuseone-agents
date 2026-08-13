package domain

import "fmt"

/*
How much of one payload is kept, and what a partial copy looks like.

Object storage is optional and an installation without it holds bulky payloads
in PostgreSQL, where there is a size past which that stops being reasonable
(PRD DE-03). Past it the store keeps a prefix. The day object storage arrives,
this is the number that goes up.

Here rather than in either store because both bound the same thing and neither
can import the other. Two copies of this rule is two rules, and the one that
drifts is the fake — which is exactly the copy every test trusts.
*/

// DefaultContentLimit is what one tool result or one set of arguments may
// occupy. Generous for JSON and small enough that a row stays a row.
const DefaultContentLimit = 1 << 20 // 1 MiB

/*
TruncationNotice is appended to what was kept.

Visible on purpose, and to every reader. The model is the one that matters:
handed half a JSON document with no notice it would reason over it as though it
were whole, which is worse than being told the answer is incomplete. An auditor
reading the stored copy learns the same thing from the same line, and the
digest beside it is still the digest of everything the tool returned.
*/
const TruncationNotice = "\n\n[truncated by FuseOne: the tool returned %d bytes and this installation stores %d]"

/*
Truncate keeps what fits and says so in the bytes it keeps.

Truncating rather than refusing: a tool call that already reached the far side
cannot be un-made by the store declining to remember its answer, and a run left
with no result at all is worse off than one holding a partial one it knows is
partial.
*/
func Truncate(data []byte, limit int) ([]byte, bool) {
	if limit <= 0 || len(data) <= limit {
		return data, false
	}
	notice := fmt.Sprintf(TruncationNotice, len(data), limit)

	// The notice fits inside the limit rather than pushing past it: a limit
	// the store's own marker could exceed is not a limit.
	//
	// A limit smaller than the notice keeps the notice and nothing else. That
	// is the honest answer at that size — there is no room for both, and of
	// the two the reader needs to know the payload is partial more than they
	// need forty bytes of it.
	if limit <= len(notice) {
		return []byte(notice[:limit]), true
	}
	return append(append([]byte{}, data[:limit-len(notice)]...), notice...), true
}
