package spec

import (
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// Missing is a field a definition must have before it publishes, and a draft
// may still lack.
type Missing string

const (
	MissingInstructions Missing = "instructions"
	MissingTools        Missing = "tools"
	MissingBudget       Missing = "budget"
	MissingProvider     Missing = "provider"
)

// missingProblems is what a publication says about each missing field.
var missingProblems = map[Missing]string{
	MissingInstructions: "the body is the agent's instructions and cannot be empty",
	MissingTools:        "tools is required; an agent with no capability pack can do nothing",
	MissingBudget:       "budget needs at least a cost or step ceiling",
	MissingProvider:     "provider is required",
}

// validate rejects a definition that cannot run safely.
//
// Everything checked here is something the platform cannot supply a sensible
// default for. A missing budget is the sharp one: without a ceiling a runaway
// agent bills until someone notices, so an unbudgeted agent does not publish
// (PRD FO-02).
func (s Spec) validate() error {
	problems := s.identityProblems()
	for _, m := range s.missing() {
		problems = append(problems, missingProblems[m])
	}
	structural, err := s.structuralProblems()
	if err != nil {
		return err
	}
	return invalidIf(append(problems, structural...))
}

// identityProblems is what a definition cannot be without, draft or not: its
// identifier, and the area its cost is attributed to.
func (s Spec) identityProblems() []string {
	var problems []string
	if s.ID == "" {
		problems = append(problems, "id is required")
	}
	if s.Area == "" {
		problems = append(problems, "area is required — it is the unit of cost attribution")
	}
	if s.Company != "" {
		if err := domain.ValidCompanyID(string(s.Company)); err != nil {
			problems = append(problems, fmt.Sprintf("company is invalid: %v", err))
		}
	}
	return problems
}

// missing is what is required for publication and not there yet, in the
// order a publication reports it.
func (s Spec) missing() []Missing {
	var out []Missing
	if s.Instructions == "" {
		out = append(out, MissingInstructions)
	}
	if len(s.Tools) == 0 {
		out = append(out, MissingTools)
	}
	if s.Budget.Micros <= 0 && s.Budget.Steps <= 0 {
		out = append(out, MissingBudget)
	}
	if s.Provider == "" {
		out = append(out, MissingProvider)
	}
	return out
}

// structuralProblems is what is wrong with what is there, however unfinished
// the rest is. An invalid approval policy is an error of its own, as it was.
func (s Spec) structuralProblems() ([]string, error) {
	var problems []string
	if s.CachePrefixTTL != "" && s.CachePrefixTTL != "5m" && s.CachePrefixTTL != "1h" {
		problems = append(problems, "cache_prefix_ttl must be 5m or 1h; the provider sells no other window")
	}
	problems = append(problems, s.fallbackProblems()...)
	problems = append(problems, triggerProblems(s.Triggers)...)
	problems = append(problems, emitProblems(s.Emits)...)
	problems = append(problems, stepProblems(s.Steps, s.Tools)...)
	if err := s.Approvals.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := s.MemoryLearning.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	if err := s.StandingApprovals.Validate(); err != nil {
		problems = append(problems, err.Error())
	}
	return problems, nil
}

func invalidIf(problems []string) error {
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

// ParseDraft reads a definition still being written (NT-018 1b).
//
// What is structurally wrong is refused as Parse refuses it, and so is a
// draft with no identifier or area: those are set by the human step at its
// birth. What is only missing for publication is listed instead.
func ParseDraft(source string, data []byte) (Spec, []Missing, error) {
	s, err := parseFields(source, data)
	if err != nil {
		return Spec{}, nil, err
	}
	structural, err := s.structuralProblems()
	if err != nil {
		return Spec{}, nil, fmt.Errorf("%s: %w", source, err)
	}
	if err := invalidIf(append(s.identityProblems(), structural...)); err != nil {
		return Spec{}, nil, fmt.Errorf("%s: %w", source, err)
	}
	return s, s.missing(), nil
}
