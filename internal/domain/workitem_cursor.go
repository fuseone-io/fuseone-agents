package domain

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

/*
Where a page of a card's trail ended.

The trail reads oldest claim first and breaks a shared instant by run id, so
a position is those two values and nothing else. Keyset rather than offset:
runs keep claiming the card while somebody reads it, and an offset would
repeat or skip whatever moved under it.

A cursor carries no authority. Every page applies the caller's own run:read,
scope and cost:read, so a cursor handed from a wide caller to a narrow one
reaches only what the narrow one may read. A token that is not exactly a
position — malformed, half, or carrying any field beside the two — is
refused rather than read as the first page: unlike the run list, a trail
reader paging through a card should be told the token is broken, not shown
its beginning again as if it were the next page.
*/

// ErrWorkItemCursorInvalid is a cursor token that is not a position.
var ErrWorkItemCursorInvalid = errors.New("work item trail cursor is not a position")

// WorkItemCursor is a position on the trail.
type WorkItemCursor struct {
	ClaimedAt time.Time `json:"t"`
	RunID     RunID     `json:"r"`
}

// WorkItemCursorAt is the position of one run on the trail.
func WorkItemCursorAt(r WorkItemRun) *WorkItemCursor {
	return &WorkItemCursor{ClaimedAt: r.ClaimedAt, RunID: r.RunID}
}

// Encode renders a position as one URL-safe token.
func (c *WorkItemCursor) Encode() string {
	if c == nil {
		return ""
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeWorkItemCursor reads a token back. No token is the first page.
func DecodeWorkItemCursor(token string) (*WorkItemCursor, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWorkItemCursorInvalid, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c WorkItemCursor
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWorkItemCursorInvalid, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: data after the position", ErrWorkItemCursorInvalid)
	}
	if c.RunID == "" || c.ClaimedAt.IsZero() {
		return nil, fmt.Errorf("%w: incomplete", ErrWorkItemCursorInvalid)
	}
	return &c, nil
}

// Precedes reports whether a run falls after this position in the trail's
// order. No cursor precedes every run.
func (c *WorkItemCursor) Precedes(r WorkItemRun) bool {
	if c == nil {
		return true
	}
	return CompareWorkItemRuns(WorkItemRun{ClaimedAt: c.ClaimedAt, RunID: c.RunID}, r) < 0
}

// DefaultWorkItemPage and MaxWorkItemPage bound how many runs one page holds.
const (
	DefaultWorkItemPage = 50
	MaxWorkItemPage     = 200
)

// WorkItemPage asks for one page of a trail.
type WorkItemPage struct {
	After *WorkItemCursor
	Limit int
}

// Size is the page's run count: the default when unset, never above the cap.
func (p WorkItemPage) Size() int {
	switch {
	case p.Limit <= 0:
		return DefaultWorkItemPage
	case p.Limit > MaxWorkItemPage:
		return MaxWorkItemPage
	}
	return p.Limit
}
