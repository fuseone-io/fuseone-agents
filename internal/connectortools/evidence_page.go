package connectortools

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// errEvidenceRefused is a query the source refused inside a 200 response.
var errEvidenceRefused = errors.New("connector: the evidence source refused the query")

// errEvidenceShape is a body that does not hold what the declaration says.
// Reading it as an empty set would report absence from a parsing failure.
var errEvidenceShape = errors.New("connector: the evidence response has an unexpected shape")

// evidencePage is one parsed response.
type evidencePage struct {
	rows   []map[string]json.RawMessage
	next   string
	digest string
}

func parseEvidencePage(cfg EvidenceConfig, body []byte) (evidencePage, error) {
	if evidenceRefused(cfg.Refusal, body) {
		return evidencePage{}, errEvidenceRefused
	}
	raw, found := evidencePath(body, cfg.Rows)
	if !found {
		return evidencePage{}, errEvidenceShape
	}
	rows, ok := evidenceRows(raw)
	if !ok {
		return evidencePage{}, errEvidenceShape
	}
	sum := sha256.Sum256(raw)
	page := evidencePage{rows: rows, digest: hex.EncodeToString(sum[:])}
	if cfg.Paging.Kind == EvidencePagingCursor {
		next, ok := evidenceNextCursor(body, cfg.Paging.NextCursor)
		if !ok {
			return evidencePage{}, errEvidenceShape
		}
		page.next = next
	}
	return page, nil
}

// evidenceRows accepts an array of objects and nothing else. Null decodes without
// an error, and it would read as a complete collection of nothing.
func evidenceRows(raw json.RawMessage) ([]map[string]json.RawMessage, bool) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil || rows == nil {
		return nil, false
	}
	for _, row := range rows {
		if row == nil {
			return nil, false
		}
	}
	return rows, true
}

// evidenceNextCursor tells the source saying it is finished, with a null or an
// empty cursor, from not saying anything readable. Only the first ends a
// collection: behind the second there may be more events.
func evidenceNextCursor(body []byte, path string) (string, bool) {
	raw, found := evidencePath(body, path)
	if !found {
		return "", false
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return "", false
	}
	switch next := value.(type) {
	case nil:
		return "", true
	case string:
		return next, true
	case json.Number:
		return next.String(), true
	default:
		return "", false
	}
}

func evidenceRefused(refusal EvidenceMatch, body []byte) bool {
	if refusal.Field == "" {
		return false
	}
	raw, found := evidencePath(body, refusal.Field)
	return found && evidenceScalar(raw) == refusal.Value
}

// evidencePath walks a dotted path through objects.
func evidencePath(body []byte, path string) (json.RawMessage, bool) {
	current := json.RawMessage(body)
	for _, segment := range strings.Split(path, ".") {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(current, &object); err != nil {
			return nil, false
		}
		next, found := object[segment]
		if !found {
			return nil, false
		}
		current = next
	}
	return current, true
}

// evidenceScalar is a JSON string or number as text; anything else is empty.
func evidenceScalar(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || isJSONNull(trimmed) {
		return ""
	}
	if trimmed[0] == '"' {
		var text string
		if json.Unmarshal(trimmed, &text) != nil {
			return ""
		}
		return text
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return ""
	}
	return string(trimmed)
}

func isJSONNull(raw []byte) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// projectEvidence keeps the declared fields and nothing else: this is the
// guarantee, whatever the source chose to send.
func projectEvidence(row map[string]json.RawMessage, fields []string) ([]byte, error) {
	kept := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		if value, found := row[field]; found {
			kept[field] = value
		}
	}
	return json.Marshal(kept)
}

// evidenceInstantOf reads an event's time: epoch seconds, epoch milliseconds or
// RFC 3339. Unreadable is no time, which only narrows what the answer can state.
func evidenceInstantOf(raw json.RawMessage) (int64, bool) {
	text := evidenceScalar(raw)
	if text == "" {
		return 0, false
	}
	if number, err := strconv.ParseFloat(text, 64); err == nil {
		if number > 1e12 {
			number /= 1000
		}
		return int64(number), true
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed.Unix(), true
	}
	return 0, false
}
