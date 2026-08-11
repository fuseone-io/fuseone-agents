package domain

import "fmt"

// Effect classifies a tool's impact on the world.
//
// Classification lives on the tool and is set by the Curator at registration
// time (PRD DE-12). The agent author never sets it: they have no way of
// knowing what is dangerous, and holding them responsible for it is precisely
// what makes open authoring unsafe.
type Effect uint8

const (
	// EffectUnknown is the zero value: an unclassified tool never executes.
	EffectUnknown Effect = iota
	EffectRead
	EffectWrite
	EffectDestructive
	EffectFinancial
)

var effectNames = map[Effect]string{
	EffectUnknown:     "unknown",
	EffectRead:        "read",
	EffectWrite:       "write",
	EffectDestructive: "destructive",
	EffectFinancial:   "financial",
}

func (e Effect) String() string {
	if n, ok := effectNames[e]; ok {
		return n
	}
	return fmt.Sprintf("effect(%d)", uint8(e))
}

func (e Effect) Valid() bool {
	_, ok := effectNames[e]
	return ok && e != EffectUnknown
}

// Reversible reports whether the effect can be undone by compensation (SE-08).
// Writes usually can; destructive and financial actions cannot, which is why
// they require human approval up front rather than rollback afterwards.
func (e Effect) Reversible() bool {
	return e == EffectRead || e == EffectWrite
}

// AtLeast compares severity without scattering switch statements everywhere.
func (e Effect) AtLeast(min Effect) bool {
	return e >= min
}

func ParseEffect(v string) (Effect, error) {
	for e, name := range effectNames {
		if name == v && e != EffectUnknown {
			return e, nil
		}
	}
	return EffectUnknown, fmt.Errorf("unknown effect: %q", v)
}
