package spec

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Render writes a specification back as the file it came from.
//
// The console does not publish a different kind of artefact from the one on
// disk: it renders this, and the result is parsed and published like any
// other file. Three things fall out of that, and all three matter.
//
// The version stays the digest of the bytes, so the same definition written in
// an editor and typed into the console produces the same version — publishing
// one after the other is a no-op rather than a second version of identical
// text.
//
// An installation that keeps its agents in git can export what the console
// wrote and commit it, and one that writes files can open them in the console.
// Neither direction is a migration.
//
// And the format stays the only representation. A console that stored fields
// directly would make the file a second source of truth the moment somebody
// edited one.
func Render(s Spec) ([]byte, error) {
	fm := frontmatter{
		ID:       string(s.ID),
		Name:     s.Name,
		Area:     string(s.Area),
		Provider: s.Provider,
		Model:    s.Model,
		Effort:   s.Effort,
		Triggers: s.Triggers,
	}
	for _, t := range s.Tools {
		fm.Tools = append(fm.Tools, string(t))
	}
	fm.Budget.Micros = s.Budget.Micros
	fm.Budget.Tokens = s.Budget.Tokens
	fm.Budget.ToolCalls = s.Budget.ToolCalls
	fm.Budget.Steps = s.Budget.Steps
	fm.Budget.WallClockMS = s.Budget.WallClockMS

	front, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("spec: render front matter: %w", err)
	}

	var out strings.Builder
	out.WriteString("---\n")
	out.Write(front)
	out.WriteString("---\n\n")
	out.WriteString(strings.TrimSpace(s.Instructions))
	out.WriteString("\n")
	return []byte(out.String()), nil
}
