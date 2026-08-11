package trigger

import "encoding/json"

// mustJSON encodes a payload whose shape is fixed at compile time. A failure
// here is a struct that cannot be marshalled, which is a bug rather than a
// runtime condition.
func mustJSON(v any) []byte {
	out, err := json.Marshal(v)
	if err != nil {
		panic("trigger: payload cannot be encoded: " + err.Error())
	}
	return out
}
