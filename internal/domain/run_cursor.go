package domain

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

/*
Where a page of the run list ended.

The list is ordered by when a run started and broken by its identifier, so a
position is those two values. Both are needed: several runs start in the same
millisecond, and a cursor holding time alone would either skip the ones sharing
an instant with the boundary or hand them back on the next page.

Offset would have been less code. It is also wrong here for a reason that shows
up exactly when it matters: the list is newest first and runs are being opened
while somebody reads it, so page two at offset fifty is not the fifty-first run
— it is whatever the first fifty have been pushed down to.

A cursor carries a position and no authority. The filter is applied to the
resumed page exactly as it was to the one before it.
*/
type RunCursor struct {
	StartedAt time.Time `json:"t"`
	RunID     RunID     `json:"r"`
}

// RunCursorAt is the position of one summary in the list.
func RunCursorAt(s RunSummary) *RunCursor {
	return &RunCursor{StartedAt: s.StartedAt, RunID: s.RunID}
}

// Encode renders a position as one URL-safe token.
func (c *RunCursor) Encode() string {
	if c == nil {
		return ""
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeRunCursor reads a token back.
//
// A token that does not parse is treated as no token rather than as an error:
// the caller pasted a URL, and answering the first page is more useful than
// refusing to answer at all.
func DecodeRunCursor(token string) *RunCursor {
	if token == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil
	}
	var c RunCursor
	if err := json.Unmarshal(raw, &c); err != nil || c.RunID == "" {
		return nil
	}
	return &c
}

// Before answers whether a summary falls past this position in the list's
// ordering: started earlier, or started at the same instant under a lower
// identifier.
func (c *RunCursor) Before(s RunSummary) bool {
	if c == nil {
		return true
	}
	if s.StartedAt.Equal(c.StartedAt) {
		return s.RunID < c.RunID
	}
	return s.StartedAt.Before(c.StartedAt)
}
