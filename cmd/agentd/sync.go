// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/fuseone/agents/internal/admin"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/trigger"
)

// What start-up reconciles between the specification files and the database.
//
// Publishing, schedules, webhooks and rulings are all the same act: the files
// are the source, the database is what the running platform reads, and
// start-up makes the second agree with the first.

// syncSchedules reconciles the trigger table with what the version the boot
// left current declares, for each agent the boot published.
//
// The version left current, not the newest file: the sync makes current each
// version it can publish, and skips one whose digest it cannot prove (NT-018).
// Reconciling from the newest file would leave an agent running one version
// with another's schedule. An agent the boot published nothing for is not in
// current, and its schedules stay as they were.
func syncSchedules(ctx context.Context, pool *pgxpool.Pool, current map[domain.AgentID]spec.Spec) error {
	schedules := trigger.NewPostgresSchedules(pool)
	now := time.Now()
	for _, agent := range slices.Sorted(maps.Keys(current)) {
		if err := schedules.Sync(ctx, agent, spec.CronSchedules(current[agent]), now); err != nil {
			return fmt.Errorf("sync schedules for %s: %w", agent, err)
		}
	}
	return nil
}

// syncWebhooks reconciles the declared paths with the version the boot left
// current, as syncSchedules does. Secrets are untouched: publishing a new
// version must not break every sender configured against a path, because
// editing a prompt is not a security event.
func syncWebhooks(ctx context.Context, pool *pgxpool.Pool, current map[domain.AgentID]spec.Spec) error {
	hooks := trigger.NewPostgresWebhooks(pool)
	for _, agent := range slices.Sorted(maps.Keys(current)) {
		published := current[agent]
		scope := domain.Scope{Area: domain.AreaID(published.Area)}
		if err := hooks.Sync(ctx, agent, scope, spec.Webhooks(published)); err != nil {
			// A path two agents both declare is a configuration error, and one
			// of them keeps it. Loud but not fatal: refusing to start would
			// take the whole installation down over one file, and the path
			// that already works keeps working.
			if errors.Is(err, trigger.ErrPathTaken) {
				slog.Error("webhook path already belongs to another agent; this one will not fire",
					"agent", agent, "err", err)
				continue
			}
			return fmt.Errorf("sync webhooks for %s: %w", agent, err)
		}
	}
	return nil
}

// webhookPathsOf picks the paths out of a specification's triggers.
// pauseNewAgents records every agent nobody has decided about as paused.
func pauseNewAgents(ctx context.Context, pool *pgxpool.Pool, specDir *string) error {
	loaded := spec.NewStore()
	if _, err := loaded.LoadDir(ctx, specsRoot(*specDir), specsPath(*specDir)); err != nil {
		return fmt.Errorf("record new agents as paused: load %s: %w", *specDir, err)
	}

	state := spec.NewState(pool)
	for _, agent := range loaded.Agents() {
		if err := state.EnsurePaused(ctx, agent, "worker"); err != nil {
			return err
		}
	}
	return nil
}

// cronSchedulesOf picks the schedules out of a specification's triggers.
func syncRulings(ctx context.Context, catalog *tools.Catalog, curator *admin.Curator) error {
	applied, err := catalog.Sync(ctx, curator, domain.Scope{})
	if err != nil {
		// Starting with every tool silently demoted to read-only would look
		// like a working worker while every write agent quietly stalls.
		return fmt.Errorf("apply tool classifications: %w", err)
	}
	slog.Info("tool classifications applied", "count", applied)
	return nil
}

// publishSpecs records every definition on disk as a published version.
//
// The worker is where definitions are read today; the Studio will write them
// directly later (PRD DE-07). Either way the registry is what the rest of the
// installation reads, so the two never disagree about what is published.
// bootPublication is what the boot's publication decided: how many versions it
// published and skipped, and, per agent, the version it left current.
type bootPublication struct {
	published, skipped int
	current            map[domain.AgentID]spec.Spec
}

func publishSpecs(ctx context.Context, registry *spec.Registry, dir string) (bootPublication, error) {
	out := bootPublication{current: map[domain.AgentID]spec.Spec{}}
	store := spec.NewStore()
	if _, err := store.LoadDir(ctx, specsRoot(dir), specsPath(dir)); err != nil {
		return out, fmt.Errorf("load agent definitions from %s: %w", dir, err)
	}

	for _, agent := range store.Agents() {
		for _, version := range store.Versions(agent) {
			s, err := store.Get(agent, version)
			if err != nil {
				return out, err
			}
			// The company is the installation's single one until phase 2
			// (PRD §3.1); the area comes from the definition itself.
			err = registry.Publish(ctx, s, "worker", auth.BootstrapScope.Company)
			if errors.Is(err, spec.ErrDigestUnknown) {
				// Published before digests were kept, so these bytes cannot be
				// shown to be the row's (NT-018). The row stays as it is, and
				// current if it was; the file needs a change to get an id of its
				// own. Not a reason to keep every other agent from starting.
				slog.Warn("agent version skipped: published without a digest",
					"agent", s.ID, "version", s.Version)
				out.skipped++
				continue
			}
			if err != nil {
				return out, err
			}
			// What this worker holds is what the installation runs. Without
			// saying so, a version published once and then withdrawn from
			// disk stays the newest by timestamp for ever, and every new run
			// pins to a specification nobody holds (PRD DE-08).
			if err := registry.MakeCurrent(ctx, s.ID, s.Version); err != nil {
				return out, err
			}
			out.published++
			out.current[s.ID] = s
		}
	}
	return out, nil
}

// loadSpecs publishes the agent definitions on disk and wires the resolver to
// the configured model providers and tool catalogue.
//
// Providers come from the installation's configuration; credentials come from
// the environment rather than the definition, so an agent file is safe to
// commit to a repository.
type toolSchemas interface {
	Schema(domain.ToolID) (name, description string, input map[string]any, ok bool)
}

func loadSpecs(
	ctx context.Context, dir string, catalog toolSchemas,
	integrations *admin.Integrations, registry *spec.Registry,
) (*spec.Resolver, error) {
	/*
		Where definitions are resolved from, which is the difference between
		the two ways this platform is installed.

		With a database the registry is the source. Publishing is an interface
		action rather than a deploy (PRD DE-07), so an agent authored in the
		console has no file anywhere — resolving from disk left every one of
		them visible on every screen, accepting triggers, and parking with
		spec_unresolved on the first turn.

		The directory is still read, and still published: it is how an
		installation seeds itself and how a laptop with no database works at
		all. It is a seed, not the source.
	*/
	var definitions spec.Definitions = registry

	store := spec.NewStore()
	loaded, err := store.LoadDir(ctx, specsRoot(dir), specsPath(dir))
	if err != nil {
		return nil, fmt.Errorf("load agent definitions from %s: %w", dir, err)
	}
	slog.Info("loaded agent definitions", "count", loaded, "dir", dir)

	if registry == nil {
		definitions = spec.Local(store)
	}

	providers := model.NewRegistry(nil)
	if err := registerConfigured(ctx, providers, integrations); err != nil {
		return nil, err
	}
	go watchConfiguration(ctx, providers, integrations)

	if len(providers.Names()) == 0 {
		slog.Warn("no model provider configured; add one in the administration area")
	}

	return spec.NewResolver(definitions, providers, catalog), nil
}

/*
specsRoot and specsPath turn a directory into the pair io/fs wants.

`io/fs` paths are unrooted by contract — a leading slash is invalid, not
absolute — so `os.DirFS(".")` with `/agents` fails with "invalid argument"
rather than reading anything. Which is what an installation mounting its agents
at an absolute path got, and the only kind of path a Kubernetes volumeMount
produces: the specs ConfigMap the chart offers could never have worked.

An absolute directory becomes its own root. A relative one keeps resolving
against the working directory, so a developer running `agentd worker` in a
checkout still gets `agents/`.
*/
func specsRoot(dir string) fs.FS {
	if filepath.IsAbs(dir) {
		return os.DirFS(dir)
	}
	return os.DirFS(".")
}

func specsPath(dir string) string {
	if filepath.IsAbs(dir) {
		return "."
	}
	return dir
}
