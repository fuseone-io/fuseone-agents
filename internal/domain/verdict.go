package domain

import "fmt"

// Verdict is the Gate's ruling on a proposed action (PRD 10.2).
type Verdict uint8

const (
	// VerdictUnknown is the zero value: a missing decision never authorises.
	VerdictUnknown Verdict = iota
	VerdictAllow
	// VerdictConstrain allows the call with modified arguments. It keeps the
	// value of the automation while containing the risk, and is usually more
	// useful than an outright denial.
	VerdictConstrain
	VerdictRequireApproval
	VerdictBlock
	// VerdictDuplicate means the Gate recognised the proposed effect as
	// already recorded. Nothing leaves the platform, but this is not a
	// refusal: the run should continue without repeating the effect.
	VerdictDuplicate
)

var verdictNames = map[Verdict]string{
	VerdictUnknown:         "unknown",
	VerdictAllow:           "allow",
	VerdictConstrain:       "constrain",
	VerdictRequireApproval: "require_approval",
	VerdictBlock:           "block",
	VerdictDuplicate:       "duplicate",
}

func (v Verdict) String() string {
	if n, ok := verdictNames[v]; ok {
		return n
	}
	return fmt.Sprintf("verdict(%d)", uint8(v))
}

// Executable reports whether the action proceeds to execution under this
// ruling. RequireApproval does not: the run suspends until a human decides,
// and the Gate is re-evaluated afterwards.
func (v Verdict) Executable() bool {
	return v == VerdictAllow || v == VerdictConstrain
}

// Terminal reports whether the ruling ends evaluation without executing.
func (v Verdict) Terminal() bool {
	return v == VerdictBlock || v == VerdictDuplicate
}

func ParseVerdict(s string) (Verdict, error) {
	for v, name := range verdictNames {
		if name == s && v != VerdictUnknown {
			return v, nil
		}
	}
	return VerdictUnknown, fmt.Errorf("unknown verdict: %q", s)
}

// Decision is the verdict plus what makes it auditable: the rule that produced
// it and the hash of the policy in force at the time (PRD AU-08). Without
// those two fields a replay can reproduce the outcome but never re-evaluate it.
type Decision struct {
	Verdict Verdict
	// Rule names the rule that applied. The trail never reads "denied by
	// policy" — it names which rule denied (PRD AU-10).
	Rule string
	// PolicyHash identifies the policy version that was evaluated.
	PolicyHash string
	// Args carries the modified arguments when Verdict == VerdictConstrain.
	Args []byte
	// Reason is the business-language explanation shown in the trail. It is
	// rendered through i18n; store a message key, not a localised sentence.
	Reason string
	// PolicyCode names which authored policy produced this, when one did.
	// Without it the trail records "policy" and nobody can tell two rules
	// apart or count what either did.
	PolicyCode string
	// Monitored are policies that matched while watching rather than
	// enforcing. Recorded so the trail can say a decision was observed and not
	// obeyed — otherwise a screen shows a rule denying things beside a run
	// that carried on, and somebody spends an afternoon on it.
	Monitored []MonitoredPolicy
	// Budget context is present when the budget rule blocks. It says which
	// dimension would cross the ceiling and with what projected consumption,
	// so a screen does not have to infer "cost" from a rule that also governs
	// tokens, calls, steps and wall clock.
	Budget    Budget
	Committed Consumption
	Estimate  Consumption
	Projected Consumption
	Breached  string
}

// MonitoredPolicy is what a watching rule would have done.
type MonitoredPolicy struct {
	Code    string  `json:"code"`
	Verdict Verdict `json:"verdict"`
	Reason  string  `json:"reason,omitempty"`
}
