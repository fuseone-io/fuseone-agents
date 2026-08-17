// Package spec parses and versions agent definitions.
//
// An agent is a Markdown file: machine-readable contract in the frontmatter,
// instructions in the body. The body is the prompt, so the thing a domain
// author reviews and the thing the model receives are the same text — there is
// no translation step in which intent can be lost.
package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fuseone/agents/internal/domain"
)

var (
	ErrNoFrontmatter = errors.New("spec: file has no frontmatter block")
	ErrInvalid       = errors.New("spec: invalid definition")
)

// Spec is one version of an agent.
type Spec struct {
	ID   domain.AgentID
	Name string
	Area domain.AreaID

	// Version is the digest of the file's bytes. Making the version *be* the
	// content means a run pinned to a version is pinned to exact text — there
	// is no way to edit a published version in place, and "did this change"
	// is a string comparison (PRD DE-08).
	Version domain.VersionID

	Provider string
	Model    string
	Effort   string

	// Tools is the capability pack, resolved to concrete tool ids. The author
	// picks a pack; this is what the pack expanded to (PRD SE-03).
	Tools  []domain.ToolID
	Budget domain.Budget

	Triggers []Trigger

	// Emits are the events this agent publishes when a run of it finishes
	// (PRD SE-10).
	//
	// Declared rather than called. If emitting were a tool the model chose,
	// the graph of who triggers whom would depend on what a model decided on
	// the day, and the requirement is that it is static and inspectable. An
	// agent names an event and never an agent: it does not know who listens,
	// which is what keeps this composition rather than a phone call.
	Emits Emits

	// Steps narrow what is reachable as a run advances. Absent means one
	// envelope holding the whole pack, which is how every agent behaved
	// before steps existed.
	Steps []Step

	// Instructions is the file body: what the agent is for, in the author's
	// own words.
	Instructions string

	// Source is where it was read from, for the console and for errors.
	Source string
}

// Trigger is what starts a run.
type Trigger struct {
	Type     string `yaml:"type"`
	Schedule string `yaml:"schedule,omitempty"`
	Path     string `yaml:"path,omitempty"`
	Event    string `yaml:"on,omitempty"`
}

// frontmatter is the wire shape of the YAML block.
type frontmatter struct {
	ID       string    `yaml:"id"`
	Name     string    `yaml:"name"`
	Area     string    `yaml:"area"`
	Provider string    `yaml:"provider"`
	Model    string    `yaml:"model"`
	Effort   string    `yaml:"effort"`
	Tools    []string  `yaml:"tools"`
	Triggers []Trigger `yaml:"triggers"`
	Emits    Emits     `yaml:"emits,omitempty"`
	Steps    []Step    `yaml:"steps,omitempty"`
	Budget   struct {
		Micros      int64 `yaml:"micros"`
		Tokens      int64 `yaml:"tokens"`
		ToolCalls   int64 `yaml:"tool_calls"`
		Steps       int64 `yaml:"steps"`
		WallClockMS int64 `yaml:"wall_clock_ms"`
	} `yaml:"budget"`
}

// Parse reads one agent definition.
func Parse(source string, data []byte) (Spec, error) {
	front, body, err := split(data)
	if err != nil {
		return Spec{}, fmt.Errorf("%s: %w", source, err)
	}

	var fm frontmatter
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return Spec{}, fmt.Errorf("%s: %w: %v", source, ErrInvalid, err)
	}

	s := Spec{
		ID:           domain.AgentID(fm.ID),
		Name:         fm.Name,
		Area:         domain.AreaID(fm.Area),
		Version:      versionOf(data),
		Provider:     fm.Provider,
		Model:        fm.Model,
		Effort:       fm.Effort,
		Triggers:     fm.Triggers,
		Emits:        fm.Emits,
		Steps:        fm.Steps,
		Instructions: strings.TrimSpace(string(body)),
		Source:       source,
		Budget: domain.Budget{
			Micros:      fm.Budget.Micros,
			Tokens:      fm.Budget.Tokens,
			ToolCalls:   fm.Budget.ToolCalls,
			Steps:       fm.Budget.Steps,
			WallClockMS: fm.Budget.WallClockMS,
		},
	}
	for _, t := range fm.Tools {
		s.Tools = append(s.Tools, domain.ToolID(t))
	}

	if err := s.validate(); err != nil {
		return Spec{}, fmt.Errorf("%s: %w", source, err)
	}
	return s, nil
}

// validate rejects a definition that cannot run safely.
//
// Everything checked here is something the platform cannot supply a sensible
// default for. A missing budget is the sharp one: without a ceiling a runaway
// agent bills until someone notices, so an unbudgeted agent does not publish
// (PRD FO-02).
func (s Spec) validate() error {
	var problems []string

	if s.ID == "" {
		problems = append(problems, "id is required")
	}
	if s.Area == "" {
		problems = append(problems, "area is required — it is the unit of cost attribution")
	}
	if s.Instructions == "" {
		problems = append(problems, "the body is the agent's instructions and cannot be empty")
	}
	if len(s.Tools) == 0 {
		problems = append(problems, "tools is required; an agent with no capability pack can do nothing")
	}
	if s.Budget.Micros <= 0 && s.Budget.Steps <= 0 {
		problems = append(problems, "budget needs at least a cost or step ceiling")
	}
	if s.Provider == "" {
		problems = append(problems, "provider is required")
	}
	problems = append(problems, triggerProblems(s.Triggers)...)
	problems = append(problems, emitProblems(s.Emits)...)

	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

// split separates the YAML frontmatter from the Markdown body.
func split(data []byte) (front, body []byte, err error) {
	text := string(data)
	// Tolerate a leading byte order mark or blank lines; an author's editor
	// may add either, and neither should make a definition unreadable.
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.TrimLeft(text, " \t\r\n")

	if !strings.HasPrefix(text, "---") {
		return nil, nil, ErrNoFrontmatter
	}
	rest := strings.TrimPrefix(text, "---")
	rest = strings.TrimLeft(rest, "\r\n")

	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, nil, ErrNoFrontmatter
	}

	front = []byte(rest[:end])
	body = []byte(strings.TrimLeft(rest[end+len("\n---"):], "\r\n"))
	return front, body, nil
}

// versionOf derives the immutable version from the file's bytes.
func versionOf(data []byte) domain.VersionID {
	sum := sha256.Sum256(data)
	return domain.VersionID("v" + hex.EncodeToString(sum[:])[:12])
}

// triggerProblems reports every trigger this platform would not act on.
//
// Reported at parse rather than discovered at run time, because the failure
// mode of an unserved trigger is silence: it publishes, the screen prints it
// back as configured, and nothing ever fires it.
func triggerProblems(triggers []Trigger) []string {
	var problems []string
	for _, t := range triggers {
		switch t.Type {
		case TriggerCron, TriggerWebhook, TriggerEvent:
		case TriggerChannel:
			// A channel trigger names nothing. A field here would be the
			// author choosing which conversation may start their agent, and
			// which conversations belong to which scope is administrative.
			if t.Schedule != "" || t.Path != "" || t.Event != "" {
				problems = append(problems,
					"a channel trigger names no conversation: which conversations may "+
						"start an agent is decided in the administration area, not in a definition")
			}
		default:
			problems = append(problems, fmt.Sprintf(
				"trigger type %q is not one this platform serves: cron, webhook, event or channel", t.Type))
		}
	}
	return problems
}
