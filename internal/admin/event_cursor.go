package admin

import (
	"encoding/base64"
	"encoding/json"
)

// eventCursor is where an administrative trail page ended.
//
// The event id is append-only and ordered by the query, so it can page without
// offsets repeating or skipping rows while new administrative changes arrive.
type eventCursor struct {
	ID int64 `json:"i"`
}

func encodeEventCursor(id int64) string {
	if id == 0 {
		return ""
	}
	raw, err := json.Marshal(eventCursor{ID: id})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeEventCursor(token string) int64 {
	if token == "" {
		return 0
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0
	}
	var c eventCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return 0
	}
	if c.ID < 0 {
		return 0
	}
	return c.ID
}
