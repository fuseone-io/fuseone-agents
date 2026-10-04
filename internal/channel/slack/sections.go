package slack

import "strings"

// maxAnswerSections bounds how many section blocks one answer may become.
// Slack refuses a message past 50 blocks; the provenance context is one more,
// and the margin keeps this render from ever being the reason a message is
// refused. The ceiling bounds the shape, not the content: the tail is merged,
// never dropped.
const maxAnswerSections = 20

// maxSectionChars is the fail-safe. The reporter already bounds the whole
// answer well under Slack's per-section limit, but that ceiling belongs to
// another layer and may move; a section past this falls back to the
// single-block render rather than producing a request Slack refuses.
const maxSectionChars = 2900

/*
answerSections renders the answer's own structure, and nothing it does not have.

Typography, never interpretation: a blank line is a paragraph break by the
text's own definition, a run of bullet lines is one list, a code fence is one
piece — so those become separate blocks with breathing room. Nothing is
guessed: no line is promoted to a headline, nothing is reordered or demoted.
The partition invariant in the tests is the contract — rejoining the sections
reconstitutes the single-block render exactly.

Each paragraph goes through outcome() on its own, which is safe because that
translation is line-based: translating the parts equals translating the whole.
*/
func answerSections(text string) []string {
	paragraphs := splitParagraphs(text)
	sections := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		rendered := outcome(paragraph)
		if rendered == "" {
			continue
		}
		sections = append(sections, rendered)
	}
	if len(sections) == 0 {
		return sections
	}
	for _, section := range sections {
		if len(section) > maxSectionChars {
			// The fail-safe: today's behaviour, never a refused request.
			return []string{outcome(text)}
		}
	}
	if len(sections) > maxAnswerSections {
		merged := strings.Join(sections[maxAnswerSections-1:], "\n\n")
		sections = append(sections[:maxAnswerSections-1], merged)
	}
	return sections
}

// splitParagraphs cuts at blank lines outside code fences. A contiguous run
// of bullet lines has no blank between them and therefore stays one
// paragraph; bullets the model spaced out are, by the text's own definition,
// separate paragraphs — and this renders structure, never repairs it.
func splitParagraphs(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var paragraphs []string
	var current []string
	inFence := false

	flush := func() {
		if len(current) > 0 {
			paragraphs = append(paragraphs, strings.Join(current, "\n"))
			current = nil
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			current = append(current, line)
			continue
		}
		if inFence {
			// Content, not a break: a fence split in half renders as two
			// broken fences, which rewrites what the agent wrote.
			current = append(current, line)
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return paragraphs
}
