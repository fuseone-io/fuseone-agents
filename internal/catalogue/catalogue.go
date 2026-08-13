/*
Package catalogue holds the agents somebody can start from (PRD FU-16).

Four recurring shapes — triaging a ticket, qualifying a lead, reconciling two
records, answering an alert — written out so an author adjusts one instead of
facing an empty page. Starting from nothing is the reason most of these never
get written.

A template is not a specification and deliberately cannot become one by itself:
it names no tools. A template that named `crm.reply` would be broken in every
installation that calls its CRM something else, and picking the pack is the
author's act anyway — they choose from what the Curator has connected, which is
the whole of SE-03. What a template carries instead is `needs`: the roles it
expects, in words, so the author knows what they are looking for in the picker.

They ship inside the binary. A catalogue an operator can edit is one that
drifts from the product, and there is nothing here worth configuring.
*/
package catalogue

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*.agent.md
var files embed.FS

// Template is a starting point, not an agent.
type Template struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	// Summary is the one line that decides whether somebody opens it.
	Summary string `yaml:"summary"`
	// Area is a suggestion. The author's own grant decides where it lands.
	Area string `yaml:"area,omitempty"`

	// Needs are the roles this agent expects to be able to reach, in the
	// author's language: "read the customer's history", not "crm.lookup".
	// The pack is chosen from what this installation actually connected.
	Needs []string `yaml:"needs,omitempty"`

	Triggers []Trigger `yaml:"triggers,omitempty"`
	Steps    []Step    `yaml:"steps,omitempty"`
	Budget   Budget    `yaml:"budget,omitempty"`

	// Instructions is the body: what the agent is for, ready to be edited.
	Instructions string `yaml:"-"`
}

type Trigger struct {
	Type     string `yaml:"type"`
	Schedule string `yaml:"schedule,omitempty"`
	Path     string `yaml:"path,omitempty"`
	Event    string `yaml:"on,omitempty"`
}

type Step struct {
	Name      string `yaml:"name"`
	StopsWhen string `yaml:"stops_when,omitempty"`
}

type Budget struct {
	Micros      int64 `yaml:"micros,omitempty"`
	Tokens      int64 `yaml:"tokens,omitempty"`
	ToolCalls   int64 `yaml:"tool_calls,omitempty"`
	Steps       int64 `yaml:"steps,omitempty"`
	WallClockMS int64 `yaml:"wall_clock_ms,omitempty"`
}

// All returns the catalogue, in a stable order.
//
// Sorted by name rather than by filename so the gallery does not reorder itself
// when somebody renames a file, and read once per call because there are four
// of them and they are in memory already.
func All() ([]Template, error) {
	entries, err := fs.ReadDir(files, "templates")
	if err != nil {
		return nil, fmt.Errorf("catalogue: read templates: %w", err)
	}

	out := make([]Template, 0, len(entries))
	for _, entry := range entries {
		raw, err := files.ReadFile(path.Join("templates", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("catalogue: read %s: %w", entry.Name(), err)
		}
		template, err := parse(entry.Name(), raw)
		if err != nil {
			return nil, err
		}
		out = append(out, template)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns one template by id.
func Get(id string) (Template, error) {
	all, err := All()
	if err != nil {
		return Template{}, err
	}
	for _, t := range all {
		if t.ID == id {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf("catalogue: no template %q", id)
}

// parse reads the same shape a definition has: YAML between fences, then the
// body. Deliberately the same, so a template is legible to anybody who has
// read an agent file — and so one can be lifted out of a real agent.
func parse(name string, raw []byte) (Template, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Template{}, fmt.Errorf("catalogue: %s does not open with a --- fence", name)
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return Template{}, fmt.Errorf("catalogue: %s has no closing fence", name)
	}

	var template Template
	if err := yaml.Unmarshal([]byte(rest[:end]), &template); err != nil {
		return Template{}, fmt.Errorf("catalogue: %s: %w", name, err)
	}
	template.Instructions = strings.TrimSpace(rest[end+len("\n---\n"):])

	switch {
	case template.ID == "":
		return Template{}, fmt.Errorf("catalogue: %s has no id", name)
	case template.Name == "" || template.Summary == "":
		return Template{}, fmt.Errorf("catalogue: %s needs a name and a summary", name)
	case template.Instructions == "":
		return Template{}, fmt.Errorf("catalogue: %s has no body", name)
	}
	return template, nil
}
