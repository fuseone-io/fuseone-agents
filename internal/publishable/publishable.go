// Package publishable answers whether a parsed agent definition may be
// published here, in this installation, today.
//
// The parser already refused what a definition says about itself. What is
// left needs the installation: whether its providers are configured for the
// agent's scope, whether the tools it names are offered there, whether its
// schedules are ones this platform can read, and whether untrusted data can
// reach a non-reversible tool. Every way of publishing asks these, in this
// order, and gets the same answer: the JSON fields, the `.agent.md` file, and
// later the workbench's validator, which refuses an operation the publication
// would refuse (NT-018, contract 6).
//
// Below the HTTP layer on purpose: a validator in internal/authoring must be
// able to ask without depending on internal/httpapi.
package publishable

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

// Providers answers whether a model provider is configured for a scope.
// Declared here by the consumer.
type Providers interface {
	Configured(name string, at domain.Scope) bool
}

// Catalogue is every tool the installation knows, with what the Curator
// ruled about each. Declared here by the consumer.
type Catalogue interface {
	Tools(ctx context.Context) ([]domain.ToolEntry, error)
}

// Checker judges a definition against the installation.
//
// A nil Providers or Catalogue is an installation with nothing wired to
// compare against — a laptop, a test — and checks nothing on that side.
type Checker struct {
	Providers Providers
	Tools     Catalogue
}

// Check is the refusal for the first check the definition fails, or "" when
// it may be published. now is the instant schedules are read from.
func (c Checker) Check(ctx context.Context, s spec.Spec, now time.Time) (string, error) {
	// Before the version is written, because a schedule nobody can parse
	// would otherwise be published, reported as saved, and never fire.
	if bad := unparseableSchedule(s, now); bad != "" {
		return fmt.Sprintf("the schedule %q is not one this platform can read", bad), nil
	}
	if detail := c.unconfiguredPair(s); detail != "" {
		return detail, nil
	}
	// One read of the catalogue for both checks, so a classification landing
	// mid-publish cannot leave them judging different catalogues.
	catalogue, err := c.Rulings(ctx, ScopeOf(s))
	if err != nil {
		return "", err
	}
	if unoffered := c.unofferedTools(s, catalogue); len(unoffered) > 0 {
		return unofferedToolsDetail(unoffered), nil
	}
	if blocked := blockingStaticFlows(s, catalogue); len(blocked) > 0 {
		return staticFlowBlockDetail(blocked), nil
	}
	return "", nil
}

// ScopeOf is where the definition will run.
func ScopeOf(s spec.Spec) domain.Scope {
	return domain.Scope{Company: s.Company, Area: s.Area}
}

// unparseableSchedule is the first schedule this platform cannot read, or "".
//
// A schedule that reached the table unparseable would be read as due forever;
// one that never reached it at all is an agent the screen calls scheduled and
// no clock has heard of.
func unparseableSchedule(s spec.Spec, now time.Time) string {
	for _, schedule := range spec.CronSchedules(s) {
		if _, err := trigger.NextAfter(schedule, now); err != nil {
			return schedule
		}
	}
	return ""
}

// unconfiguredPair is the refusal for either pair a version names: the main
// one first, then its fallback.
func (c Checker) unconfiguredPair(s spec.Spec) string {
	if detail := c.unconfiguredProvider(s); detail != "" {
		return detail
	}
	return c.unconfiguredFallback(s)
}

/*
unconfiguredProvider is the refusal for a main pair whose provider is not
configured for the agent's scope (NT-019, phase 3): another area's, or nobody's.

Configured rather than usable: a provider switched off, or whose key this
process could not open, is still configured, and the worker's planner refuses
a run on it. The detail names the provider and never where else it is
configured.
*/
func (c Checker) unconfiguredProvider(s spec.Spec) string {
	if c.Providers == nil || s.Provider == "" {
		return ""
	}
	if c.Providers.Configured(s.Provider, ScopeOf(s)) {
		return ""
	}
	return fmt.Sprintf("the provider %q is not configured for this agent's scope", s.Provider)
}

/*
unconfiguredFallback is the refusal for a fallback nobody can ask.

Checked at publish rather than found out at run time, because the moment it
would otherwise be found out is the outage the fallback exists for.
*/
func (c Checker) unconfiguredFallback(s spec.Spec) string {
	if s.Fallback.IsZero() || c.Providers == nil {
		return ""
	}
	if c.Providers.Configured(s.Fallback.Provider, ScopeOf(s)) {
		return ""
	}
	return fmt.Sprintf("the fallback provider %q is not one this installation configures",
		s.Fallback.Provider)
}

// unofferedTools is every tool in a draft's pack this installation does not
// offer, for the area it publishes into: unknown to the catalogue, or
// discovered and kept off the surface.
//
// Either way the tool is never described to a model nor invoked, so an agent
// published holding one stops at its first call — and a typo copied into both
// `tools` and a step's reaches passes every check that only compares the two.
// A tool on the surface the Curator has not ruled on is not here: that is a
// server just connected, and publishing before its classification is
// legitimate. Platform tools are served by the platform, not listed by any
// server. With no catalogue wired there is nothing to compare against.
func (c Checker) unofferedTools(s spec.Spec, catalogue Rulings) []domain.ToolID {
	if c.Tools == nil {
		return nil
	}
	var unoffered []domain.ToolID
	for _, tool := range s.Tools {
		if entry, known := catalogue[tool]; (!known || !entry.OnSurface) && !tool.PlatformOwned() {
			unoffered = append(unoffered, tool)
		}
	}
	return unoffered
}

func unofferedToolsDetail(tools []domain.ToolID) string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = string(tool)
	}
	return fmt.Sprintf(
		"this installation does not offer %s: correct the name, or connect the server that "+
			"provides it and bring the tool onto the surface before publishing",
		strings.Join(names, ", "))
}
