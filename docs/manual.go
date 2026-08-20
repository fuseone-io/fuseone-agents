/*
Package docs carries the manual into the binary.

The package sits beside the documents rather than importing them from
somewhere tidier because go:embed cannot reach outside its own directory. The
alternative is a build step that copies docs/ into a Go package, and a copy
step that does not run ships a console whose manual is empty — a failure with
no symptom until a reader opens it. Here the file a reviewer reads in the pull
request is the byte for byte file the console serves, and there is no second
copy to drift.

Only manual/ is embedded. The PRD and the notes argue decisions with the people
who make them; they are not written for the person the console is for.
*/
package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

//go:embed all:manual
var manual embed.FS

// Page is one document of the manual, as the console needs it.
//
// Body is Markdown, not HTML: rendering belongs to the reader, and a server
// that hands out HTML has to be trusted about what it put in it.
type Page struct {
	Slug     string
	Title    string
	Summary  string
	Section  string
	Tags     []string
	Order    int
	Headings []Heading
	Body     string
}

// Heading is one h2/h3 entry the console can use for a table of contents and
// search without carrying each page body in the index.
type Heading struct {
	ID    string
	Title string
	Level int
}

/*
Manual reads every page, in the order the index shows them.

An error rather than a page with empty fields: a manual is content, and content
with a missing title is a bug in the pull request that added it, not a gap for
the console to render around.
*/
func Manual(locale string) ([]Page, error) {
	entries, err := fs.Glob(manual, "manual/"+locale+"/*.md")
	if err != nil {
		return nil, fmt.Errorf("manual: %w", err)
	}

	pages := make([]Page, 0, len(entries))
	for _, name := range entries {
		raw, err := fs.ReadFile(manual, name)
		if err != nil {
			return nil, fmt.Errorf("manual %s: %w", name, err)
		}
		page, err := parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("manual %s: %w", name, err)
		}
		page.Slug = strings.TrimSuffix(strings.TrimPrefix(name, "manual/"+locale+"/"), ".md")
		pages = append(pages, page)
	}

	sort.Slice(pages, func(i, j int) bool { return pages[i].Order < pages[j].Order })
	return pages, nil
}

// front is the fence a page opens with. Deliberately not YAML: three keys do
// not justify a dependency, and a parser that accepts only what the manual uses
// refuses a page that tries to say something the console cannot show.
const front = "---\n"

func parse(raw string) (Page, error) {
	rest, found := strings.CutPrefix(raw, front)
	if !found {
		return Page{}, fmt.Errorf("no front matter: a page opens with %q", strings.TrimSpace(front))
	}
	head, body, found := strings.Cut(rest, front)
	if !found {
		return Page{}, fmt.Errorf("front matter is not closed")
	}

	var page Page
	// Tracked rather than inferred from the value: 0 is a position somebody can
	// legitimately choose, so "is it zero" cannot answer "did they say".
	var ordered bool
	for _, line := range strings.Split(strings.TrimSpace(head), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Page{}, fmt.Errorf("front matter line %q is not key: value", line)
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "title":
			page.Title = value
		case "summary":
			page.Summary = value
		case "section":
			page.Section = value
		case "tags":
			page.Tags = splitTags(value)
		case "order":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Page{}, fmt.Errorf("order %q is not a number: %w", value, err)
			}
			page.Order, ordered = n, true
		default:
			return Page{}, fmt.Errorf("front matter key %q is not one the console reads", key)
		}
	}

	if page.Title == "" || page.Summary == "" || page.Section == "" {
		return Page{}, fmt.Errorf("title, summary and section are required")
	}
	if len(page.Tags) == 0 {
		return Page{}, fmt.Errorf("tags are required: they are what search can match before loading a page")
	}
	// The contract declares order required, and a page that omits it does not
	// fail — it takes position zero and quietly becomes the first thing anybody
	// reads. Uniqueness cannot catch that while only one page forgets.
	if !ordered {
		return Page{}, fmt.Errorf("order is required: it decides where the index puts this page")
	}
	page.Body = strings.TrimLeft(body, "\n")
	page.Headings = headings(page.Body)
	return page, nil
}

func splitTags(value string) []string {
	var tags []string
	for _, tag := range strings.Split(value, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func headings(body string) []Heading {
	var out []Heading
	seen := make(map[string]int)
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		level, title, ok := headingLine(trimmed)
		if !ok {
			continue
		}
		id := anchorID(title)
		seen[id]++
		if seen[id] > 1 {
			id = fmt.Sprintf("%s-%d", id, seen[id])
		}
		out = append(out, Heading{ID: id, Title: title, Level: level})
	}
	return out
}

func headingLine(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level < 2 || level > 3 || len(line) <= level || line[level] != ' ' {
		return 0, "", false
	}
	title := strings.TrimSpace(line[level+1:])
	title = strings.TrimSpace(strings.TrimRight(title, "#"))
	return level, title, title != ""
}

func anchorID(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	id := b.String()
	if id == "" {
		return "section"
	}
	return id
}
