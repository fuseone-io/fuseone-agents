package domain

import "strings"

// Sentence renders a policy as the line an author reads back.
//
// Generated from the same fields the Gate evaluates, never written separately.
// The builder is not the only representation on the screen — the operator
// always sees the sentence the engine will run — and two representations
// maintained apart will disagree, at which point the screen is lying about
// what the platform does.
//
//	crm.* · escrita · args.rows > 100 · data.taint em untrusted → negar
func (p Policy) Sentence() string {
	parts := []string{resourceLabel(p.Resource)}

	if len(p.Effects) > 0 {
		effects := make([]string, 0, len(p.Effects))
		for _, effect := range p.Effects {
			effects = append(effects, effect.String())
		}
		parts = append(parts, strings.Join(effects, ", "))
	}

	for _, condition := range p.Conditions {
		parts = append(parts, condition.Sentence())
	}

	sentence := strings.Join(parts, " · ") + " → " + effectLabel(p.Effect)
	if p.Mode == PolicyMonitor {
		// Said in the sentence, not only in a badge elsewhere on the screen.
		// A rule that reads "→ deny" while denying nothing is the single most
		// misleading thing this screen could show.
		sentence += " (apenas monitorando)"
	}
	return sentence
}

// Sentence renders one clause.
func (c Condition) Sentence() string {
	return c.Field + " " + operatorLabel(c.Op) + " " + c.Value
}

func resourceLabel(resource string) string {
	if resource == "" {
		return "*"
	}
	return resource
}

// Deliberately the operator symbols an author typed, not prose: `>` reads
// faster than "maior que" in a line meant to be scanned, and the dropdown
// beside it already spells the choice out.
var operatorLabels = map[string]string{
	OpEquals:      "=",
	OpNotEquals:   "≠",
	OpGreaterThan: ">",
	OpLessThan:    "<",
	OpContains:    "contém",
	OpIn:          "em",
}

func operatorLabel(op string) string {
	if label, ok := operatorLabels[op]; ok {
		return label
	}
	// An operator nobody implemented reads as itself rather than vanishing.
	// The rule does not hold either, and the two have to agree.
	return op
}

var effectLabels = map[PolicyEffect]string{
	PolicyAllow:    "permitir",
	PolicyEscalate: "escalar",
	PolicyDeny:     "negar",
}

func effectLabel(effect PolicyEffect) string {
	if label, ok := effectLabels[effect]; ok {
		return label
	}
	return string(effect)
}
