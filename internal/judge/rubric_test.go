package judge

import (
	"strings"
	"testing"
)

/*
A rubric is the judge's instructions and a declared output (NT-016 § The rubric).

Its version is its digest, so the declaration is part of what is hashed, a reason
included. The values are a closed list the platform presents as fixed-width codes, so the
judge never writes the value itself.
*/

func TestParseRubric_validDeclarations_areAccepted(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"pass_fail": `{"instructions":"Is the answer polite?","output":{"type":"pass_fail"}}`,
		"score":     `{"instructions":"Rate completeness.","output":{"type":"score","min":0,"max":10,"threshold":7}}`,
		"category":  `{"instructions":"Classify.","output":{"type":"category","categories":["ok","high_risk"],"passing":["ok"]},"reason":true}`,
		// Any integers, as long as there are at most eleven values (NT-016).
		"far from zero":          `{"instructions":"x","output":{"type":"score","min":2000,"max":2010,"threshold":2005}}`,
		"ending at the top":      `{"instructions":"x","output":{"type":"score","min":9223372036854775797,"max":9223372036854775807,"threshold":9223372036854775807}}`,
		"starting at the bottom": `{"instructions":"x","output":{"type":"score","min":-9223372036854775808,"max":-9223372036854775798,"threshold":-9223372036854775808}}`,
	} {
		if _, err := ParseRubric([]byte(raw)); err != nil {
			t.Errorf("%s: ParseRubric = %v, want accepted", name, err)
		}
	}
}

func TestParseRubric_invalidDeclarations_areRefused(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"no instructions":       `{"instructions":"","output":{"type":"pass_fail"}}`,
		"unknown type":          `{"instructions":"x","output":{"type":"json"}}`,
		"unknown field":         `{"instructions":"x","output":{"type":"pass_fail"},"weight":2}`,
		"twelve scores":         `{"instructions":"x","output":{"type":"score","min":0,"max":11,"threshold":5}}`,
		"min not below max":     `{"instructions":"x","output":{"type":"score","min":3,"max":3,"threshold":3}}`,
		"threshold above max":   `{"instructions":"x","output":{"type":"score","min":1,"max":5,"threshold":6}}`,
		"one category":          `{"instructions":"x","output":{"type":"category","categories":["ok"],"passing":["ok"]}}`,
		"eleven categories":     `{"instructions":"x","output":{"type":"category","categories":["a","b","c","d","e","f","g","h","i","j","k"],"passing":["a"]}}`,
		"prose category":        `{"instructions":"x","output":{"type":"category","categories":["looks fine","bad"],"passing":["bad"]}}`,
		"repeated category":     `{"instructions":"x","output":{"type":"category","categories":["ok","ok"],"passing":["ok"]}}`,
		"no passing category":   `{"instructions":"x","output":{"type":"category","categories":["ok","bad"]}}`,
		"undeclared passing":    `{"instructions":"x","output":{"type":"category","categories":["ok","bad"],"passing":["great"]}}`,
		"categories on a score": `{"instructions":"x","output":{"type":"score","min":1,"max":5,"threshold":3,"categories":["a","b"]}}`,
		"range on pass_fail":    `{"instructions":"x","output":{"type":"pass_fail","max":5}}`,
		"trailing data":         `{"instructions":"x","output":{"type":"pass_fail"}} {}`,
		// Presence is what the contract declares: a zero is a value, absence is none.
		"score without threshold":   `{"instructions":"x","output":{"type":"score","min":0,"max":5}}`,
		"score without min":         `{"instructions":"x","output":{"type":"score","max":5,"threshold":3}}`,
		"pass_fail with min zero":   `{"instructions":"x","output":{"type":"pass_fail","min":0}}`,
		"category with min zero":    `{"instructions":"x","output":{"type":"category","categories":["ok","bad"],"passing":["ok"],"min":0}}`,
		"pass_fail with categories": `{"instructions":"x","output":{"type":"pass_fail","categories":[]}}`,
		"score with passing":        `{"instructions":"x","output":{"type":"score","min":0,"max":5,"threshold":3,"passing":[]}}`,
		// A range that overflows, and one that ends where a loop would wrap.
		"overflowing range": `{"instructions":"x","output":{"type":"score","min":-9223372036854775808,"max":9223372036854775807,"threshold":0}}`,
		"twelve at the top": `{"instructions":"x","output":{"type":"score","min":9223372036854775796,"max":9223372036854775807,"threshold":9223372036854775807}}`,
		// null is not absence: a field written as null was written.
		"null min on pass_fail":        `{"instructions":"x","output":{"type":"pass_fail","min":null}}`,
		"null min on a category":       `{"instructions":"x","output":{"type":"category","categories":["ok","bad"],"passing":["ok"],"min":null}}`,
		"null categories on pass_fail": `{"instructions":"x","output":{"type":"pass_fail","categories":null}}`,
		"null categories on a score":   `{"instructions":"x","output":{"type":"score","min":0,"max":5,"threshold":3,"categories":null}}`,
		"null reason":                  `{"instructions":"x","output":{"type":"pass_fail"},"reason":null}`,
		"null output":                  `{"instructions":"x","output":null}`,
		"duplicate type":               `{"instructions":"x","output":{"type":"score","type":"pass_fail"}}`,
		"duplicate instructions":       `{"instructions":"x","instructions":"y","output":{"type":"pass_fail"}}`,
		"null inside categories":       `{"instructions":"x","output":{"type":"category","categories":["ok",null],"passing":["ok"]}}`,
	} {
		if _, err := ParseRubric([]byte(raw)); err == nil {
			t.Errorf("%s: accepted, want refused", name)
		}
	}
}

// The digest is the rubric's version: the same declaration however it is spelled, and a
// new one for any change, asking for a reason included.
func TestRubricDigest_namesTheDeclarationNotItsSpelling(t *testing.T) {
	t.Parallel()
	parse := func(raw string) Rubric {
		t.Helper()
		r, err := ParseRubric([]byte(raw))
		if err != nil {
			t.Fatalf("ParseRubric: %v", err)
		}
		return r
	}
	base := parse(`{"instructions":"Polite?","output":{"type":"pass_fail"}}`)
	respaced := parse("{ \"output\" : {\"type\":\"pass_fail\"},\n \"instructions\":\"Polite?\" }")
	withReason := parse(`{"instructions":"Polite?","output":{"type":"pass_fail"},"reason":true}`)
	reworded := parse(`{"instructions":"Polite? ","output":{"type":"pass_fail"}}`)

	if base.Digest() != respaced.Digest() {
		t.Error("the same declaration spelled differently has two digests")
	}
	if base.Digest() == withReason.Digest() {
		t.Error("asking for a reason did not change the digest")
	}
	if base.Digest() == reworded.Digest() {
		t.Error("changing the instructions did not change the digest")
	}
	if !strings.HasPrefix(base.Digest(), "sha256:") || len(base.Digest()) != len("sha256:")+64 {
		t.Errorf("digest = %q, want a full sha256", base.Digest())
	}
}

// A rubric nobody validated has no options rather than a loop: Options is public, and a
// range that wraps would never end.
func TestOptions_ofAnUnvalidatedRubric_areNone(t *testing.T) {
	t.Parallel()
	top := int(^uint(0) >> 1)
	low := -top - 1
	r := Rubric{Instructions: "x", Output: Output{Type: OutputScore, Min: &low, Max: &top, Threshold: &top}}
	if got := r.Options(); got != nil {
		t.Errorf("options = %d, want none", len(got))
	}
}

// The widest range at the top of the integers walks its eleven values and stops.
func TestOptions_aRangeEndingAtTheTop_endsThere(t *testing.T) {
	t.Parallel()
	r := mustRubric(t, `{"instructions":"x","output":{"type":"score","min":9223372036854775797,"max":9223372036854775807,"threshold":9223372036854775807}}`)
	got := r.Options()
	if len(got) != 11 || got[10].Value != "9223372036854775807" || got[10].Code != 'K' || !got[10].Passes {
		t.Errorf("options = %+v, want eleven ending at the top", got)
	}
}

func TestOptions_areFixedWidthCodesInTheRubricsOrder(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		raw   string
		want  []Option
		label string
	}{
		{`{"instructions":"x","output":{"type":"pass_fail"}}`,
			[]Option{{'A', "pass", true}, {'B', "fail", false}}, "pass_fail"},
		{`{"instructions":"x","output":{"type":"score","min":1,"max":3,"threshold":2}}`,
			[]Option{{'A', "1", false}, {'B', "2", true}, {'C', "3", true}}, "score"},
		{`{"instructions":"x","output":{"type":"category","categories":["ok","high_risk"],"passing":["ok"]}}`,
			[]Option{{'A', "ok", true}, {'B', "high_risk", false}}, "category"},
	} {
		r, err := ParseRubric([]byte(c.raw))
		if err != nil {
			t.Fatalf("%s: %v", c.label, err)
		}
		got := r.Options()
		if len(got) != len(c.want) {
			t.Fatalf("%s: options = %+v, want %+v", c.label, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: option %d = %+v, want %+v", c.label, i, got[i], c.want[i])
			}
		}
	}
}

// Only the end of the input may follow the value. The rubric's second decoder catches
// trailing data today; the walk refuses it on its own, so nothing depends on that.
func TestStrictJSON_onlyTheEndMayFollowTheValue(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{} x`, `{}}`, `{} {}`, `[] ]`} {
		if err := strictJSON([]byte(raw)); err == nil {
			t.Errorf("strictJSON(%q) accepted, want refused", raw)
		}
	}
	if err := strictJSON([]byte("{} \n\t")); err != nil {
		t.Errorf("whitespace after the value refused: %v", err)
	}
}
