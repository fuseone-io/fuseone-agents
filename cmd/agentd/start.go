package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

// startCmd opens a run so a worker can pick it up.
//
// The same path the console, the scheduler and the webhook take. It used to
// append the opening step itself, which made it the one door that did not
// honour a paused agent — a pause obeyed by three of four ways to start
// something is not a pause.
func startCmd(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	agent := fs.String("agent", "", "agent id, as written in its definition")
	// The definition comes from the registry now, not from disk: the console
	// and the schedulers all read what was published, and a run started from
	// a file nobody published would be a run pinned to a version the rest of
	// the installation cannot see.
	runID := fs.String("run", "", "idempotency key; one is generated per invocation when empty")
	by := fs.String("by", "cli", "who is starting this run, for the trail")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *agent == "" {
		return errors.New("start: --agent is required")
	}

	ctx := context.Background()

	store, err := openStore(ctx, *dsn)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("start: connect: %w", err)
	}
	defer pool.Close()

	// The version is resolved from the registry rather than accepted as an
	// argument: a run pinned to a version nobody published is a run nobody
	// can reproduce.
	opener := trigger.NewOpener(store, spec.NewRegistry(pool), engine.SystemClock{}).
		WithContent(ledger.NewContent(pool)).
		WithPauses(spec.NewState(pool))

	key := *runID
	if key == "" {
		// One intention per invocation. A caller who wants the retry to reach
		// the same run passes --run, which is what that flag now means.
		key = fmt.Sprintf("cli:%s:%d", *agent, time.Now().UnixNano())
	}

	opened, err := opener.Open(ctx, trigger.Request{
		Agent:   domain.AgentID(*agent),
		IdemKey: key,
		Trigger: "cli",
		By:      domain.UserID(*by),
	})
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}

	fmt.Printf("%s  agent=%s created=%v\n", opened.RunID, *agent, opened.Created)
	return nil
}
