package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/testdb"
)

/*
A refresh of N resources is N reads by name, so each read has to cost the same
however many others exist. Counted from the router serve mounts, against the
database: a read that fetched the whole collection in one query and filtered it
in memory issues the same number of commands, so the rows it was handed are
counted too.
*/

// tracedCommand is one statement that reached the database.
type tracedCommand struct {
	sql  string
	args []any
	rows int64
}

type commandTrace struct {
	mu       sync.Mutex
	commands []*tracedCommand
}

type tracedCommandKey struct{}

func (c *commandTrace) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	one := &tracedCommand{sql: data.SQL, args: data.Args}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.commands = append(c.commands, one)
	return context.WithValue(ctx, tracedCommandKey{}, one)
}

func (c *commandTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	one, ok := ctx.Value(tracedCommandKey{}).(*tracedCommand)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	one.rows = data.CommandTag.RowsAffected()
}

func (c *commandTrace) take() []*tracedCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.commands
	c.commands = nil
	return out
}

func newTracedWorld(t *testing.T) (*readBackWorld, *commandTrace, *pgxpool.Pool) {
	t.Helper()
	testdb.Pool(t) // Migrated, by the helper that knows how.
	config, err := pgxpool.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	trace := &commandTrace{}
	config.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return readBackWorldOn(t, pool), trace, pool
}

// readByName describes one kind of resource to this test.
type readByName struct {
	name string
	path func(n string) string
	// seed creates the resource called n, with whatever a read of it has to
	// fetch, and cleans it up afterwards.
	seed func(t *testing.T, w *readBackWorld, n string)
}

func readsByName() []readByName {
	return []readByName{{
		name: "policy",
		path: func(n string) string { return "/policies/" + n },
		seed: func(t *testing.T, w *readBackWorld, n string) {
			t.Helper()
			path := "/policies/" + n
			w.removeAtTheEnd(t, path)
			w.put(t, path, map[string]any{"name": n, "effect": "deny", "mode": "monitor", "enabled": false})
		},
	}, {
		name: "mcp",
		path: func(n string) string { return "/admin/integrations/mcp-servers/" + n },
		seed: func(t *testing.T, w *readBackWorld, n string) {
			t.Helper()
			path := "/admin/integrations/mcp-servers/" + n
			w.removeAtTheEnd(t, path)
			w.put(t, path, map[string]any{
				"transport": "http", "url": "https://mcp.example.test/mcp", "token": "secret-of-" + n,
			})
		},
	}, {
		name: "provider",
		path: func(n string) string { return "/admin/integrations/providers/" + n },
		seed: func(t *testing.T, w *readBackWorld, n string) {
			t.Helper()
			path := "/admin/integrations/providers/" + n
			w.removeAtTheEnd(t, path)
			w.put(t, path, map[string]any{
				"kind": "openai_compatible", "baseUrl": "https://models.example.test", "apiKey": "secret-of-" + n,
			})
		},
	}}
}

// others is many enough that anything proportional to them cannot hide.
const others = 25

func TestReadByName_manyOthersExist_costsWhatItCostsAlone(t *testing.T) {
	for _, r := range readsByName() {
		t.Run(r.name, func(t *testing.T) {
			world, trace, _ := newTracedWorld(t)
			target := "qcount-target-" + r.name
			r.seed(t, world, target)

			measure := func() (commands, rows int64) {
				t.Helper()
				trace.take()
				world.get(t, r.path(target))
				for _, one := range trace.take() {
					commands++
					rows += one.rows
				}
				return commands, rows
			}
			aloneCommands, aloneRows := measure()
			if aloneCommands == 0 || aloneRows == 0 {
				t.Fatalf("alone: %d commands, %d rows; the counter counts nothing", aloneCommands, aloneRows)
			}

			var names []string
			for i := range others {
				other := fmt.Sprintf("qcount-other-%s-%d", r.name, i)
				names = append(names, other)
				r.seed(t, world, other)
			}
			manyCommands, manyRows := measure()
			if manyCommands != aloneCommands {
				t.Errorf("commands: %d alone, %d with %d others", aloneCommands, manyCommands, others)
			}
			if manyRows != aloneRows {
				t.Errorf("rows: %d alone, %d with %d others", aloneRows, manyRows, others)
			}

			trace.take()
			world.get(t, r.path(target))
			assertAsksOnlyAboutTheTarget(t, trace.take(), names)
		})
	}
}

// openingASecret matches a statement fetching the sealed column itself, not
// asking whether it is there.
var openingASecret = regexp.MustCompile(`(?i)select\s+(.*,\s*)?secret\s*,`)

// No statement names another resource, and any that fetches ciphertext names
// the target: that is what keeps another connection's secret unopened, since
// nothing can open what was never fetched.
func assertAsksOnlyAboutTheTarget(t *testing.T, commands []*tracedCommand, others []string) {
	t.Helper()
	for _, one := range commands {
		for _, arg := range one.args {
			text, ok := arg.(string)
			if ok && slices.Contains(others, text) {
				t.Errorf("a read asked about %q: %s", text, one.sql)
			}
		}
		if openingASecret.MatchString(one.sql) && !namesTarget(one.args) {
			t.Errorf("fetched ciphertext without naming the target: %s %v", one.sql, one.args)
		}
	}
}

func namesTarget(args []any) bool {
	for _, arg := range args {
		if text, ok := arg.(string); ok && strings.HasPrefix(text, "qcount-target-") {
			return true
		}
	}
	return false
}

// One command is not constant work if the database reads a whole table to answer
// it. Sequential scans switched off, a plan still showing one has no usable index.
func TestReadByName_everyCommand_isAnsweredFromAnIndex(t *testing.T) {
	for _, r := range readsByName() {
		t.Run(r.name, func(t *testing.T) {
			world, trace, pool := newTracedWorld(t)
			target := "qcount-target-" + r.name
			r.seed(t, world, target)
			trace.take()
			world.get(t, r.path(target))
			commands := trace.take()

			conn, err := pool.Acquire(t.Context())
			if err != nil {
				t.Fatalf("acquire: %v", err)
			}
			defer conn.Release()
			if _, err := conn.Exec(t.Context(), `set enable_seqscan = off`); err != nil {
				t.Fatalf("set: %v", err)
			}
			defer func() { _, _ = conn.Exec(context.Background(), `reset enable_seqscan`) }()

			for _, one := range commands {
				plan := explain(t, conn, one)
				if strings.Contains(plan, "Seq Scan") {
					t.Errorf("no index answers: %s\n%s", one.sql, plan)
				}
			}
		})
	}
}

func explain(t *testing.T, conn *pgxpool.Conn, one *tracedCommand) string {
	t.Helper()
	rows, err := conn.Query(t.Context(), "explain "+one.sql, one.args...)
	if err != nil {
		t.Fatalf("explain %s: %v", one.sql, err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		plan.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("explain %s: %v", one.sql, err)
	}
	return plan.String()
}
