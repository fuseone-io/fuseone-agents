package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestCanonicalCallArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  string
		right string
		equal bool
	}{
		{name: "object key order", left: `{"search":"erro","limit":10}`, right: `{"limit":10,"search":"erro"}`, equal: true},
		{name: "insignificant whitespace", left: " { \"search\" : \"erro\" } \n", right: `{"search":"erro"}`, equal: true},
		{name: "nested objects", left: `{"filter":{"b":2,"a":1}}`, right: `{"filter":{"a":1,"b":2}}`, equal: true},
		{name: "array order matters", left: `{"ids":[1,2]}`, right: `{"ids":[2,1]}`, equal: false},
		{name: "large integers stay exact", left: `{"id":9007199254740992}`, right: `{"id":9007199254740993}`, equal: false},
		{name: "numeric spelling stays distinct", left: `{"value":1}`, right: `{"value":1.0}`, equal: false},
		{name: "non JSON remains opaque", left: "opaque  bytes", right: "opaque bytes", equal: false},
		{name: "trailing JSON remains opaque", left: `{"a":1} {"b":2}`, right: `{"a":1}{"b":2}`, equal: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			left := string(domain.CanonicalCallArguments([]byte(tc.left)))
			right := string(domain.CanonicalCallArguments([]byte(tc.right)))
			if got := left == right; got != tc.equal {
				t.Fatalf("equal = %v, want %v\nleft:  %q\nright: %q", got, tc.equal, left, right)
			}
		})
	}
}

func TestCanonicalCallArguments_doesNotAliasOpaqueInput(t *testing.T) {
	t.Parallel()

	raw := []byte("not json")
	canonical := domain.CanonicalCallArguments(raw)
	raw[0] = 'X'
	if string(canonical) != "not json" {
		t.Fatalf("canonical = %q, want an owned copy", canonical)
	}
}
