package domain

import (
	"strings"
	"testing"
)

func TestNewRevision_isAStrongQuotedVersionedValidator(t *testing.T) {
	t.Parallel()
	got := string(NewRevision("policy", []byte(`{"a":1}`)))
	if !strings.HasPrefix(got, `"v1-`) || !strings.HasSuffix(got, `"`) {
		t.Fatalf("revision = %s, want a quoted strong tag with its format version", got)
	}
}

func TestNewRevision_movesWithEveryPartOfItsInput(t *testing.T) {
	t.Parallel()
	base := NewRevision("policy", []byte(`{}`), "fp1")
	for name, other := range map[string]Revision{
		"another kind":        NewRevision("provider", []byte(`{}`), "fp1"),
		"another config":      NewRevision("policy", []byte(`{"a":1}`), "fp1"),
		"another secret":      NewRevision("policy", []byte(`{}`), "fp2"),
		"no secret":           NewRevision("policy", []byte(`{}`)),
		"a secret reassigned": NewRevision("policy", []byte(`{}`), "", "fp1"),
	} {
		if other == base {
			t.Errorf("%s gave the same revision", name)
		}
	}
	if again := NewRevision("policy", []byte(`{}`), "fp1"); again != base {
		t.Errorf("the same input gave %s and %s", base, again)
	}
}

// The boundaries between the parts are part of the input: otherwise a config ending in
// what a kind begins with would name the same revision as another pair.
func TestNewRevision_partsCannotBeShiftedAcrossTheirBoundaries(t *testing.T) {
	t.Parallel()
	if NewRevision("ab", []byte("c")) == NewRevision("a", []byte("bc")) {
		t.Fatal("two different kind and config pairs share a revision")
	}
	if NewRevision("k", []byte("c"), "de", "f") == NewRevision("k", []byte("c"), "d", "ef") {
		t.Fatal("two different fingerprint lists share a revision")
	}
}

func ifMatch(t *testing.T, header string) IfMatch {
	t.Helper()
	parsed, err := ParseIfMatch(&header)
	if err != nil {
		t.Fatalf("ParseIfMatch(%q): %v", header, err)
	}
	return parsed
}

func TestIfMatch_evaluatesAsRFC9110Says(t *testing.T) {
	t.Parallel()
	const current = Revision(`"v1-abc"`)
	cases := []struct {
		name, header string
		exists, want bool
	}{
		{"the current tag", `"v1-abc"`, true, true},
		{"another tag", `"v1-old"`, true, false},
		{"in a list", `"v1-old", "v1-abc"`, true, true},
		{"in a list with no spaces", `"v1-old","v1-abc"`, true, true},
		{"in a list with empty elements", `, "v1-old",, "v1-abc" ,`, true, true},
		{"none of a list", `"v1-old", "v1-older"`, true, false},
		{"a comma inside a tag is the tag's", `"v1-abc,x"`, true, false},
		{"the current tag, weakened by a compressing proxy", `W/"v1-abc"`, true, true},
		{"another tag, weakened", `W/"v1-old"`, true, false},
		{"weak beside a strong match", `W/"v1-x", "v1-abc"`, true, true},
		{"star on something that exists", `*`, true, true},
		{"star on nothing", `*`, false, false},
		{"star with spaces", `  *  `, false, false},
		{"a tag on nothing", `"v1-abc"`, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := ifMatch(t, c.header).Allows(c.exists, current); got != c.want {
				t.Fatalf("Allows = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIfMatch_absent_allowsEverything(t *testing.T) {
	t.Parallel()
	absent, err := ParseIfMatch(nil)
	if err != nil {
		t.Fatalf("ParseIfMatch(nil): %v", err)
	}
	if !absent.Allows(true, `"v1-abc"`) || !absent.Allows(false, "") {
		t.Fatal("no header is a precondition on nothing")
	}
	var zero IfMatch
	if !zero.Allows(true, `"v1-abc"`) {
		t.Fatal("the zero value has to mean no precondition")
	}
}

// Read as absent, a malformed header would turn a client's protection off silently.
func TestParseIfMatch_malformed_isRefused(t *testing.T) {
	t.Parallel()
	for _, header := range []string{
		``, `   `, `,`, `v1-abc`, `"v1-abc`, `v1-abc"`, `W/v1-abc`, `w/"v1-abc"`,
		`*, "v1-abc"`, `"v1-abc", *`, `**`, `"a" "b"`, `"a"b`, "\"a\tb\"", `"a"b"`,
	} {
		if _, err := ParseIfMatch(&header); err == nil {
			t.Errorf("ParseIfMatch(%q) accepted it", header)
		}
	}
}
