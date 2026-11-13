package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
)

/*
A classification's level is said or left out, never null (NT-019).

Hand-written beside the generated code, which only decodes the body: there a
level left out and a level sent as null are both a nil pointer, and only the
first means the platform's ceiling. Taken as the default, a client's null would
write a ruling stronger than the owner's. So null is refused while decoding,
and the strict handler answers it as an invalid request; regeneration leaves
this file alone.
*/

// ErrNullRulingLevel is a classification body whose level is null.
var ErrNullRulingLevel = errors.New("level is owner or platform, or left out for the platform's; null is neither")

func (b *ClassifyToolJSONRequestBody) UnmarshalJSON(data []byte) error {
	var probe struct {
		Level json.RawMessage `json:"level"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(probe.Level), []byte("null")) {
		return ErrNullRulingLevel
	}
	var body ClassifyToolJSONBody
	if err := json.Unmarshal(data, &body); err != nil {
		return err
	}
	*b = ClassifyToolJSONRequestBody(body)
	return nil
}
