package ledger

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// The grammar of a no-transaction migration: what it may hold, read once
// into what the runner acts on.

/*
splitStatements cuts a marked file into statements — each ends at a line
ending in a semicolon, comment lines never end one, and a chunk of only
comments is no statement — and parses every one before any runs, so a file
outside the grammar fails having touched nothing.
*/
func splitStatements(body string) ([]indexStatement, error) {
	texts, err := cutStatements(body)
	if err != nil {
		return nil, err
	}
	out := make([]indexStatement, 0, len(texts))
	for _, text := range texts {
		parsed, err := parseIndexStatement(text)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

// indexStatement is one statement a marked file may run, as the one parser
// read it: the fence and the runner both take it from here, so what was
// allowed and what is run cannot disagree.
type indexStatement struct {
	create bool   // create index concurrently if not exists, else drop
	name   string // the index, plain: unquoted, unqualified, as PostgreSQL folds it
	table  string // a create's table: name or schema.name, unquoted, folded
	text   string // the statement as written
}

/*
indexGrammar is the whole of what a marked file may hold, and only in forms
that are safe to run again after a crash:

	create index concurrently if not exists <name> on <table> …
	drop index concurrently if exists <name>

The name is plain — [a-z_][a-z0-9_]* after folding, unquoted, unqualified —
and the table is a plain name or schema.name in the same form. The index
lands in its table's schema, so the runner resolves the table and checks
exactly that schema for an INVALID leftover. Without "if [not] exists" a re-run after a crash fails on a
duplicate or a missing index; a quoted or qualified name is one the check
would not find. A unique index is refused: a failed unique build leaves an
INVALID index that still enforces uniqueness on writes, and nothing here
needs one. Keywords match in any case, as PostgreSQL reads them; the
statement is lowered and its spaces collapsed before matching.
*/
var indexGrammar = regexp.MustCompile(
	`^(?:(create) index concurrently if not exists ([a-z_][a-z0-9_]*) on ([a-z_][a-z0-9_]*(?:\.[a-z_][a-z0-9_]*)?)(?: |\()|(drop) index concurrently if exists ([a-z_][a-z0-9_]*) ?;?$)`)

// dollarQuote matches any dollar quote, $$ or named ($body$): quoting is
// refused rather than parsed, since a quoted body could hide any statement.
var dollarQuote = regexp.MustCompile(`\$[A-Za-z_]*\$`)

var errDollarQuote = errors.New("a no-transaction migration may not use dollar quoting")

func parseIndexStatement(text string) (indexStatement, error) {
	if dollarQuote.MatchString(text) {
		return indexStatement{}, errDollarQuote
	}
	normal := normalised(text)
	m := indexGrammar.FindStringSubmatch(normal)
	switch {
	case m == nil:
		return indexStatement{}, fmt.Errorf("a no-transaction migration may only hold "+
			"create index concurrently if not exists <name> on <table> … or drop index concurrently if exists <name>, "+
			"with plain names; not %q", normal)
	case m[1] == "create":
		return indexStatement{create: true, name: m[2], table: m[3], text: text}, nil
	default:
		return indexStatement{name: m[5], text: text}, nil
	}
}

// normalised is a statement's code, lower case, with runs of space as one.
func normalised(statement string) string {
	return strings.ToLower(strings.Join(strings.Fields(code(statement)), " "))
}

func cutStatements(body string) ([]string, error) {
	var out []string
	var current []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		current = append(current, line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") || !strings.HasSuffix(trimmed, ";") {
			continue
		}
		out = append(out, strings.Join(current, "\n"))
		current = nil
	}
	if code(strings.Join(current, "\n")) != "" {
		return nil, fmt.Errorf("a no-transaction migration ends in an unterminated statement")
	}
	return out, nil
}

// code is a statement without its comment lines.
func code(statement string) string {
	var kept []string
	for _, line := range strings.Split(statement, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, " ")
}
