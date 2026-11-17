package draft

import (
	"slices"

	"github.com/fuseone/agents/internal/spec"
)

func stepIndex(d *Draft, name string) int {
	return slices.IndexFunc(d.Spec.Steps, func(s spec.Step) bool { return s.Name == name })
}

// AddStep appends a step.
type AddStep struct{ Step spec.Step }

func (AddStep) Kind() string     { return "add_step" }
func (AddStep) Class() Class     { return Shared }
func (AddStep) Fields() []string { return []string{"steps"} }
func (o AddStep) apply(d *Draft) error {
	if o.Step.Name == "" || stepIndex(d, o.Step.Name) >= 0 {
		return invalidOp(o.Kind(), "step %q is unnamed or already declared", o.Step.Name)
	}
	d.Spec.Steps = append(d.Spec.Steps, o.Step)
	return nil
}

// EditStep replaces the step of that name, which may be renamed.
type EditStep struct {
	Name string
	Step spec.Step
}

func (EditStep) Kind() string     { return "edit_step" }
func (EditStep) Class() Class     { return Shared }
func (EditStep) Fields() []string { return []string{"steps"} }
func (o EditStep) apply(d *Draft) error {
	i := stepIndex(d, o.Name)
	if i < 0 || o.Step.Name == "" || (o.Step.Name != o.Name && stepIndex(d, o.Step.Name) >= 0) {
		return invalidOp(o.Kind(), "no step %q, or %q is taken", o.Name, o.Step.Name)
	}
	d.Spec.Steps[i] = o.Step
	return nil
}

// RemoveStep takes the step of that name out.
type RemoveStep struct{ Name string }

func (RemoveStep) Kind() string     { return "remove_step" }
func (RemoveStep) Class() Class     { return Shared }
func (RemoveStep) Fields() []string { return []string{"steps"} }
func (o RemoveStep) apply(d *Draft) error {
	i := stepIndex(d, o.Name)
	if i < 0 {
		return invalidOp(o.Kind(), "no step %q", o.Name)
	}
	d.Spec.Steps = slices.Delete(d.Spec.Steps, i, i+1)
	return nil
}
