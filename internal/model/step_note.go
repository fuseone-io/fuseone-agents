package model

import (
	"fmt"

	"github.com/fuseone/agents/internal/engine"
)

/*
stepNote tells the model which stage it is in and what its author said would
end it. Nothing is inferred from the response: the model must name the
exception through the finish tool for the trail to record it.

It also says what finish means, because inside "you are at step X" the loop
contract's "nothing left to do" is true at the end of every step, and a model
reading carefully finished there — with a summary promising later steps that
nothing would ever execute (#155). Agents without steps never receive this
note and keep the contract unchanged.
*/
func stepNote(in engine.PlanInput) string {
	const runNotStep = "Finishing ends the whole run, not the step: nothing runs " +
		"after the finish tool. When this step's work is done and later steps " +
		"remain, move forward by calling a tool a later step reaches."
	if in.StopsWhen == "" {
		return fmt.Sprintf("You are at the step called %q. %s", in.Step, runNotStep)
	}
	return fmt.Sprintf(
		"You are at the step called %q. %s\n"+
			"Its author wrote that this step stops the run when: %s\n"+
			"If that has happened, call the finish tool with `stopped_by` set to "+
			"exactly `%s`, then explain in `summary` in your own words.",
		in.Step, runNotStep, in.StopsWhen, in.StopsWhen)
}
