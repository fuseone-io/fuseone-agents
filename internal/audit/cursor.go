package audit

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

/*
Where a page of the trail ended.

One position per record, not one for the trail. The two records are merged by
time, so a page boundary falls in a different place in each of them: a cursor
holding a single position would either re-read whatever the other record had
already returned or skip past it, and an audit trail that drops entries between
pages is not one.

The value is opaque to the caller and carries no authority. It says where to
resume, never what may be read — the scope filter is applied to the page that
resumes just as it was to the page before it, so a cursor obtained under a wide
grant reaches nothing under a narrow one.
*/
type Cursor struct {
	Ledger *LedgerMark `json:"l,omitempty"`
	Admin  *AdminMark  `json:"a,omitempty"`
}

// LedgerMark is the last chained entry a page carried. The run identifies the
// chain and the sequence the step within it, because a sequence alone repeats
// across runs.
type LedgerMark struct {
	At  time.Time `json:"t"`
	Run string    `json:"r"`
	Seq int64     `json:"s"`
}

// AdminMark is the last administrative entry a page carried, by its row.
type AdminMark struct {
	At time.Time `json:"t"`
	ID int64     `json:"i"`
}

// Empty answers whether there is anywhere left to resume from.
func (c Cursor) Empty() bool { return c.Ledger == nil && c.Admin == nil }

// Encode renders a cursor as one URL-safe token.
func (c Cursor) Encode() string {
	if c.Empty() {
		return ""
	}
	raw, err := json.Marshal(c)
	if err != nil {
		// Two timestamps and two strings. If this cannot be marshalled the
		// process has bigger problems than a missing page.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeCursor reads a token back.
//
// A token that does not parse is treated as no token rather than as an error:
// the caller pasted a URL, and answering the first page is more useful than
// refusing to answer at all.
func DecodeCursor(token string) Cursor {
	if token == "" {
		return Cursor{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}
	}
	return c
}
