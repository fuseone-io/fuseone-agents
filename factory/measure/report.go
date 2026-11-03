package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// render prints the measures as markdown. Same measures, same bytes: rows
// come in a fixed order and nothing depends on map iteration.
func render(w io.Writer, m Measures) {
	p := &printer{w: w}
	last := m.Window.End.Add(-24 * time.Hour)
	p.line("# Factory board measures")
	p.line("")
	p.linef("%s, project %d. Weeks %s to %s (Monday 00:00 UTC), read at %s.",
		m.Repo, m.Project, day(m.Window.Start), day(last), m.Now.UTC().Format(time.RFC3339))
	p.line("")
	p.line("Read from the board's status changes and the timelines of its issues and pull requests;")
	p.line("factory/measure/README.md defines every measure. A signal names who caught what a role")
	p.line("passed. It is not a defect count.")
	p.lanes(m)
	p.roles(m)
	p.signals(m)
	p.open(m)
	p.unmapped(m)
}

type printer struct{ w io.Writer }

func (p *printer) line(s string) { _, _ = io.WriteString(p.w, s+"\n") }

func (p *printer) linef(f string, a ...any) { p.line(fmt.Sprintf(f, a...)) }

func (p *printer) section(title string, intro ...string) {
	p.line("")
	p.line("## " + title)
	p.line("")
	for _, l := range intro {
		p.line(l)
	}
	if len(intro) > 0 {
		p.line("")
	}
}

// table prints a markdown table; align holds one letter per column, l for
// text and r for a number.
func (p *printer) table(head, align string, rows []string) {
	if len(rows) == 0 {
		p.line("None.")
		return
	}
	p.line(head)
	rule := "|"
	for _, a := range align {
		rule += map[rune]string{'l': "---|", 'r': "---:|"}[a]
	}
	p.line(rule)
	for _, r := range rows {
		p.line(r)
	}
}

// cell escapes a pipe in text the board controls, such as a lane's name, so
// it cannot split a row.
func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func day(t time.Time) string { return t.UTC().Format("2006-01-02") }

func hours(d time.Duration) string { return fmt.Sprintf("%.1f", d.Hours()) }

func (m Measures) weekLabel(wk time.Time) string {
	if !m.Now.Before(wk) && m.Now.Before(wk.Add(week)) {
		return day(wk) + " (partial)"
	}
	return day(wk)
}

func (p *printer) lanes(m Measures) {
	p.section("Lane time", "Closed stays, each counted in the week it ended, in hours. Median and p90 are nearest-rank.")
	var rows []string
	for _, r := range m.Lanes {
		rows = append(rows, fmt.Sprintf("| %s | %s | %d | %s | %s |",
			m.weekLabel(r.Week), cell(r.Lane), r.Stays, hours(r.Median), hours(r.P90)))
	}
	p.table("| Week | Lane | Stays | Median h | p90 h |", "llrrr", rows)
}

func (p *printer) roles(m Measures) {
	p.section("Per role, per week",
		"**Owned h**: the owner's share of the closed stays that ended that week. **Moves**: the",
		"board moves it made that week. **Caught later**: the signals that week against what it passed.")
	var rows []string
	for _, r := range m.Roles {
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %d | %d |",
			m.weekLabel(r.Week), r.Owner, hours(r.Owned), r.Moves, r.Caught))
	}
	p.table("| Week | Owner | Owned h | Moves | Caught later |", "llrrr", rows)
}

func (p *printer) signals(m Measures) {
	p.section("Caught-later signals", "Each counted in the week of the act that caught it.")
	var rows []string
	for _, r := range m.Signals {
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %d |", m.weekLabel(r.Week), r.Owner, cell(r.Kind), r.Count))
	}
	p.table("| Week | Owner | Signal | Count |", "lllr", rows)
}

func (p *printer) open(m Measures) {
	p.section("Open stays", fmt.Sprintf("Cards still in a lane, in no percentile above. Age to %s, in hours.",
		m.Now.UTC().Format(time.RFC3339)))
	var rows []string
	for _, r := range m.Open {
		rows = append(rows, fmt.Sprintf("| %s | %d | %s |", cell(r.Lane), r.Open, hours(r.Oldest)))
	}
	p.table("| Lane | Open | Oldest h |", "lrr", rows)
}

func (p *printer) unmapped(m Measures) {
	p.section("Unmapped actors",
		"Actors in the window that are neither a person nor a bot in factory/measure/bots.yaml.",
		"Nothing they did is attributed to a role.")
	var rows []string
	for _, r := range m.Unmapped {
		login, kind := r.Login, r.Type
		if login == "" {
			login = "(no account)"
		}
		if kind == "" {
			kind = "-"
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %d |", login, kind, r.Seen, r.Events))
	}
	p.table("| Login | Type | Seen as | Events |", "lllr", rows)
}
