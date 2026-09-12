package connectortools

import "fmt"

// SecretValue is authority used by a native connector and has no printable
// form. Its bytes stay package-private so a response or log cannot acquire them
// by adding this value to a struct.
type SecretValue struct {
	value string
}

func (s SecretValue) String() string               { return redacted }
func (s SecretValue) Format(f fmt.State, _ rune)   { _, _ = f.Write([]byte(redacted)) }
func (s SecretValue) GoString() string             { return redacted }
func (s SecretValue) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

func (s SecretValue) empty() bool { return s.value == "" }
