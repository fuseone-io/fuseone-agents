package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// startCmd opens a run so a worker can pick it up.
//
// Triggers — cron, webhook, event — are what will do this in the product. This
// is the same act reduced to its essence: append the opening step and let the
// queue do the rest. It exists so an operator can exercise an agent without
// waiting for a schedule, and it is deliberately the only way to start a run
// by hand, so "who started this" is always answerable from the trail.
func startCmd(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	agent := fs.String("agent", "", "agent id, as written in its definition")
	specDir := fs.String("specs", "agents", "directory of *.agent.md definitions")
	company := fs.String("company", "default", "company the run belongs to")
	runID := fs.String("run", "", "run id; generated when empty")
	by := fs.String("by", "cli", "who is starting this run, for the trail")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *agent == "" {
		return errors.New("start: --agent is required")
	}

	ctx := context.Background()

	// The version is resolved from the definition on disk rather than accepted
	// as an argument: a run pinned to a version nobody published is a run
	// nobody can reproduce.
	specs := spec.NewStore()
	if _, err := specs.LoadDir(ctx, os.DirFS("."), *specDir); err != nil {
		return fmt.Errorf("start: load definitions from %s: %w", *specDir, err)
	}
	versions := specs.Versions(domain.AgentID(*agent))
	if len(versions) == 0 {
		return fmt.Errorf("start: no definition for agent %q in %s", *agent, *specDir)
	}
	published, err := specs.Get(domain.AgentID(*agent), versions[len(versions)-1])
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}

	store, err := openStore(ctx, *dsn)
	if err != nil {
		return err
	}

	id := domain.RunID(*runID)
	if id == "" {
		id = domain.RunID(fmt.Sprintf("run_%s_%d", published.ID, time.Now().UnixMilli()))
	}

	step, err := store.Append(ctx, domain.Step{
		RunID:      id,
		Kind:       domain.StepRunStarted,
		Scope:      domain.Scope{Company: domain.CompanyID(*company), Area: published.Area},
		AgentID:    published.ID,
		VersionID:  published.Version,
		OnBehalfOf: domain.UserID(*by),
		At:         time.Now(),
	})
	if err != nil {
		return fmt.Errorf("start: open run: %w", err)
	}

	fmt.Printf("%s  agent=%s version=%s seq=%d\n", id, published.ID, published.Version, step.Seq)
	return nil
}
