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
