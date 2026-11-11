package judge

import "strings"

/*
Parse reads a judge's answer against the rubric, and false is a failed judgment.

Exact, never forgiving. With the reason off, the answer is one code and nothing else: no
space, no newline, no reason. With it on, the code, a newline and a reason that is not
blank. Trimming or accepting a valid prefix would let an answer that wrote more than the
rubric allows count as a verdict.
*/
func (r Rubric) Parse(answer string) (Option, bool) {
	code := answer
	if r.Reason {
		head, reason, found := strings.Cut(answer, "\n")
		if !found || strings.TrimSpace(reason) == "" {
			return Option{}, false
		}
		code = head
	}
	if len(code) != 1 {
		return Option{}, false
	}
	for _, o := range r.Options() {
		if o.Code == code[0] {
			return o, true
		}
	}
	return Option{}, false
}

// ReasonOf is the reason in an answer the rubric asked to justify, or nothing.
func (r Rubric) ReasonOf(answer string) string {
	if !r.Reason {
		return ""
	}
	_, reason, _ := strings.Cut(answer, "\n")
	return reason
}
