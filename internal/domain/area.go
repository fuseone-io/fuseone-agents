package domain

import (
	"fmt"
	"strings"
	"time"
)

// maxAreaID is generous for a department name and short enough to read in a
// table cell beside an agent's name.
const maxAreaID = 40

// RegisteredScope is a scope somebody declared, as opposed to one inferred
// from a row that happens to name it.
//
// Before the registry an area existed only as text typed into an agent, which
// made `financeiro` and `Financeiro` two areas that never meet: a ceiling set
// on one governs no agent filed under the other, and nothing reports it. The
// registry exists so the set of areas is something the platform knows rather
// than something it reconstructs.
type RegisteredScope struct {
	Scope     Scope
	Label     string
	CreatedAt time.Time
	CreatedBy UserID
}

// NormalizeAreaID folds a typed area name to the single form the platform
// stores, or refuses it.
//
// Refuses rather than escapes: the id reaches a URL path segment and a webhook
// route, and one that has to be escaped to be addressed is one somebody will
// address wrongly.
func NormalizeAreaID(typed string) (AreaID, error) {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(typed)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '_', r == '-':
			b.WriteRune('-')
		default:
			// Accents fold to the letter underneath, so "crédito" and
			// "credito" are one area rather than two.
			folded, ok := fold[r]
			if !ok {
				return "", fmt.Errorf("area %q: %q cannot appear in an identifier", typed, r)
			}
			b.WriteRune(folded)
		}
	}

	id := strings.Trim(collapse(b.String()), "-")
	if id == "" {
		return "", fmt.Errorf("area %q: an area needs a name", typed)
	}
	if len(id) > maxAreaID {
		return "", fmt.Errorf("area %q: longer than %d characters", typed, maxAreaID)
	}
	return AreaID(id), nil
}

// collapse turns runs of hyphens into one, so "risco  de credito" does not
// become "risco--de-credito".
func collapse(s string) string {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

// fold maps the accented letters of the languages the console ships in onto
// the plain letter. Built once rather than pulled from x/text: the set is
// small, closed, and a transliteration library would decide more than this.
var fold = foldTable("áàâãäå:a çč:c éèêë:e íìîï:i ñ:n óòôõö:o úùûü:u ýÿ:y")

func foldTable(spec string) map[rune]rune {
	table := map[rune]rune{}
	for _, group := range strings.Fields(spec) {
		accented, plain, _ := strings.Cut(group, ":")
		// Only the lowercase forms: the input is folded to lower before it
		// reaches here, so an uppercase entry would be unreachable.
		for _, r := range accented {
			table[r] = rune(plain[0])
		}
	}
	return table
}
