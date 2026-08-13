package authoring

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
What the assistant is asked, as text rather than as concatenation.

A prompt is the part of this package that gets rewritten most and reviewed
least, and one built by twenty WriteString calls is one nobody reads end to
end. In a file it diffs like prose, which is what it is.

One set per language, for the same reason the template catalogue has one: the
author answers in their own words, and the reply is asked to keep those words.
Wrapping English answers in a Portuguese instruction is a mismatch the model
pays for, and "in their own words" is exactly the instruction most likely to be
the casualty.
*/

//go:embed all:prompts
var prompts embed.FS

// DefaultLocale is what a request naming none, or naming one this installation
// does not ship, is prompted in.
const DefaultLocale = "pt-BR"

// organisePrompt is the main ask: the answers, turned into fields.
func organisePrompt(locale string, a Answers, catalogue []domain.ToolID) (string, error) {
	return render(locale, "organise.txt", struct {
		Answers   Answers
		Catalogue []domain.ToolID
	}{a, catalogue})
}

// placePrompt asks, on its own, which step the exception belongs to.
func placePrompt(locale, exception string, steps []spec.Step) (string, error) {
	return render(locale, "place.txt", struct {
		Exception string
		Steps     []spec.Step
	}{exception, steps})
}

func render(locale, name string, data any) (string, error) {
	raw, err := prompts.ReadFile(path.Join("prompts", resolveLocale(locale), name))
	if err != nil {
		return "", fmt.Errorf("authoring: read prompt %s: %w", name, err)
	}
	parsed, err := template.New(name).Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("authoring: parse prompt %s: %w", name, err)
	}

	var out strings.Builder
	if err := parsed.Execute(&out, data); err != nil {
		return "", fmt.Errorf("authoring: render prompt %s: %w", name, err)
	}
	return out.String(), nil
}

// resolveLocale is which set of prompts answers a request: an exact match or
// the default. No negotiation beyond that — the console knows which language
// the author is writing in and says so.
func resolveLocale(locale string) string {
	if locale == "" {
		return DefaultLocale
	}
	if _, err := fs.Stat(prompts, path.Join("prompts", locale)); err != nil {
		return DefaultLocale
	}
	return locale
}
