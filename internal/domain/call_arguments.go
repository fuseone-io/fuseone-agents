package domain

import (
	"bytes"
	"encoding/json"
	"io"
)

// CanonicalCallArguments returns the stable representation used to identify a
// tool call. JSON object order and insignificant whitespace do not change what
// a tool receives, so they must not turn one call into two. Numbers remain
// json.Number values: encoding/json then preserves their spelling, keeping 1
// distinct from 1.0 and integers above 2^53 exact.
//
// Tool transports may also accept opaque, non-JSON payloads. Those stay byte
// exact rather than being rejected or guessed at here.
func CanonicalCallArguments(raw []byte) []byte {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return append([]byte(nil), raw...)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return append([]byte(nil), raw...)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return append([]byte(nil), raw...)
	}
	return canonical
}
