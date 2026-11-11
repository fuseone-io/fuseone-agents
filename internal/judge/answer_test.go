package judge

import "testing"

/*
The judge's answer is closed (NT-016 § What the ledger holds).

Only a value of the declared shape counts. Everything else is a failed judgment, never a
score and never a default. With the reason off, the code alone: a space, a newline or a
reason after it is refused, not trimmed and not cut back to a valid prefix.
*/

func mustRubric(t *testing.T, raw string) Rubric {
	t.Helper()
	r, err := ParseRubric([]byte(raw))
	if err != nil {
		t.Fatalf("ParseRubric: %v", err)
	}
	return r
}

const categoryRubric = `{"instructions":"Classify.","output":{"type":"category","categories":["ok","high_risk"],"passing":["ok"]}}`

func TestParse_reasonOff_acceptsTheCodeAlone(t *testing.T) {
	t.Parallel()
	r := mustRubric(t, categoryRubric)
	got, ok := r.Parse("B")
	if !ok || got != (Option{'B', "high_risk", false}) {
		t.Errorf("Parse(B) = %+v ok=%v, want high_risk", got, ok)
	}
}

func TestParse_reasonOff_refusesAnythingElse(t *testing.T) {
	t.Parallel()
	r := mustRubric(t, categoryRubric)
	// The values a permissive parser would accept.
	for _, answer := range []string{
		"", " ", "B ", " B", "B\n", "\nB", "B\tbecause", "B because it is risky",
		"B\nbecause", "AB", "A B", "b", "C", "high_risk", "ok", "Option B", "\"B\"",
	} {
		if got, ok := r.Parse(answer); ok {
			t.Errorf("Parse(%q) = %+v, want a failed judgment", answer, got)
		}
	}
}

func TestParse_reasonOn_wantsTheCodeThenAReason(t *testing.T) {
	t.Parallel()
	r := mustRubric(t, `{"instructions":"Classify.","output":{"type":"category","categories":["ok","high_risk"],"passing":["ok"]},"reason":true}`)
	if got, ok := r.Parse("A\nThe answer quotes the policy."); !ok || got.Value != "ok" {
		t.Errorf("Parse(code, reason) = %+v ok=%v, want ok", got, ok)
	}
	for _, answer := range []string{"A", "A\n", "A\n   ", " A\nreason", "A \nreason", "A reason", "Z\nreason"} {
		if got, ok := r.Parse(answer); ok {
			t.Errorf("Parse(%q) = %+v, want a failed judgment", answer, got)
		}
	}
}

func TestReasonOf_isTheTextAfterTheCode_onlyWhenAsked(t *testing.T) {
	t.Parallel()
	on := mustRubric(t, `{"instructions":"x","output":{"type":"pass_fail"},"reason":true}`)
	if got := on.ReasonOf("A\nPolite throughout."); got != "Polite throughout." {
		t.Errorf("ReasonOf = %q", got)
	}
	off := mustRubric(t, `{"instructions":"x","output":{"type":"pass_fail"}}`)
	if got := off.ReasonOf("A"); got != "" {
		t.Errorf("ReasonOf with the reason off = %q, want none", got)
	}
}

// A score's values are digits, one character like a code. The judge answers the code; a
// value written instead is the shape the platform exists to prevent.
func TestParse_aScoreWrittenAsItsValue_isRefused(t *testing.T) {
	t.Parallel()
	r := mustRubric(t, `{"instructions":"x","output":{"type":"score","min":0,"max":9,"threshold":5}}`)
	for _, value := range []string{"0", "3", "9"} {
		if got, ok := r.Parse(value); ok {
			t.Errorf("Parse(%q) = %+v, want refused: the answer is the code, not the value", value, got)
		}
	}
	if got, ok := r.Parse("D"); !ok || got.Value != "3" {
		t.Errorf("Parse(D) = %+v ok=%v, want the value 3", got, ok)
	}
}
