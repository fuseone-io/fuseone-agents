// Package authoring holds the choice of model the interview uses.
//
// An agent's provider is a capability the installation grants: it decides what
// agents may reach, and it carries policy and per-run cost. The authoring
// model is a tool of the platform — it never touches a customer system, and it
// only produces text a person approves. Different decisions, taken by
// different people, so the choice lives here rather than beside the agents'.
//
// The connection does not. Integrações stays the single owner of endpoints and
// credentials, and this is a pointer into it: a second credential store would
// be a second place to leak from, to rotate, and to audit.
package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// KindAuthoring is the settings kind this package owns.
const KindAuthoring settings.Kind = "authoring"

// name is the single row: an installation has one authoring assistant.
const name = "assistant"

// ErrNoProvider means the choice named a provider nobody connected.
var ErrNoProvider = errors.New("authoring: no provider connected under that name")

// ErrNoCeiling means the assistant was configured without a daily bound.
//
// This is the only place the platform spends money outside a run — no Gate, no
// ledger, no per-run ceiling — so the bound is part of configuring it rather
// than something to add afterwards. Without it, authoring would be the single
// invisible spend in a product whose argument is that nothing is invisible.
var ErrNoCeiling = errors.New("authoring: the assistant needs a daily ceiling")

// Choice is which connected provider writes the drafts.
type Choice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort,omitempty"`

	// DailyMicros bounds what the assistant may spend in a day. It survives
	// being switched off: resetting it there would make turning the assistant
	// back on unbounded until somebody noticed.
	DailyMicros int64 `json:"dailyMicros"`

	// Enabled reports whether an installation has an authoring assistant at
	// all. False is a supported state, not a broken one: an air-gapped install
	// with no strong model still publishes agents through the form, and the
	// interview is the good path rather than the only one.
	Enabled bool `json:"-"`
}

type Store struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewStore(pool *pgxpool.Pool, s *settings.Store) *Store {
	return &Store{pool: pool, settings: s}
}

// Current answers with the choice in force, or a disabled one.
func (s *Store) Current(ctx context.Context) (Choice, error) {
	got, err := s.settings.Get(ctx, settings.ScopeInstallation, domain.Scope{}, KindAuthoring, name)
	if errors.Is(err, settings.ErrNotFound) {
		return Choice{}, nil
	}
	if err != nil {
		return Choice{}, fmt.Errorf("authoring: read: %w", err)
	}

	var choice Choice
	if err := json.Unmarshal(got.Value, &choice); err != nil {
		return Choice{}, fmt.Errorf("authoring: decode: %w", err)
	}
	choice.Enabled = got.Enabled
	return choice, nil
}

// Choose points the assistant at a connected provider.
//
// It refuses a name Integrações does not know, which is what keeps this a
// pointer. Without the check the two could drift apart silently, and the
// failure would surface as an authoring call to an endpoint that does not
// exist — at the moment somebody is halfway through describing a process.
func (s *Store) Choose(ctx context.Context, choice Choice, by domain.UserID) error {
	if choice.Provider == "" || choice.Model == "" {
		return errors.New("authoring: the assistant needs a provider and a model")
	}
	if choice.DailyMicros <= 0 {
		return ErrNoCeiling
	}

	connected, err := s.settings.List(ctx, settings.KindModelProvider)
	if err != nil {
		return fmt.Errorf("authoring: read providers: %w", err)
	}
	if !has(connected, choice.Provider) {
		return fmt.Errorf("%w: %s", ErrNoProvider, choice.Provider)
	}

	return s.write(ctx, choice, true, by, "authoring.changed")
}

// Disable turns the assistant off without forgetting which provider it used.
func (s *Store) Disable(ctx context.Context, by domain.UserID) error {
	current, err := s.Current(ctx)
	if err != nil {
		return err
	}
	return s.write(ctx, current, false, by, "authoring.disabled")
}

// write stores the choice and records it in the same transaction.
//
// Together, always. The assistant writes drafts a person then publishes, so
// which model wrote them is part of how a published agent came to say what it
// says. A change that reached the settings table without reaching the trail
// would leave that unanswerable.
func (s *Store) write(
	ctx context.Context, choice Choice, enabled bool, by domain.UserID, action string,
) error {
	value, err := json.Marshal(choice)
	if err != nil {
		return fmt.Errorf("authoring: encode: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("authoring: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.settings.PutTx(ctx, tx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      KindAuthoring,
		Name:      name,
		Value:     value,
		Enabled:   enabled,
		UpdatedBy: string(by),
	}); err != nil {
		return err
	}
	if err := admin.Record(ctx, tx, admin.Event{
		Principal: by, Action: action, Target: name,
		Detail: map[string]string{"provider": choice.Provider, "model": choice.Model},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func has(connected []settings.Setting, name string) bool {
	for _, s := range connected {
		if s.Name == name && s.Enabled {
			return true
		}
	}
	return false
}

// RecordSpend appends what an authoring call cost.
//
// The trail rather than a table of its own: authoring is the only spend that
// happens outside a run, so the append-only record an operator already reads
// is the one place it can be counted from without inventing a second ledger
// for a single figure.
func (s *Store) RecordSpend(ctx context.Context, cost domain.Cost, by domain.UserID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("authoring: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := admin.Record(ctx, tx, admin.Event{
		Principal: by, Action: "authoring.spent", Target: name,
		Detail: map[string]int64{"micros": cost.Micros},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SpentToday is what authoring has cost since midnight.
//
// Bounded to the day because a ceiling that counted every day since the
// installation started would stop the assistant for good after one busy
// afternoon, which is a different product decision than the one anybody made.
func (s *Store) SpentToday(ctx context.Context) (int64, error) {
	var micros int64
	err := s.pool.QueryRow(ctx, `
        select coalesce(sum((detail->>'micros')::bigint), 0)
        from admin_events
        where action = 'authoring.spent' and at >= date_trunc('day', now())`).Scan(&micros)
	if err != nil {
		return 0, fmt.Errorf("authoring: read spend: %w", err)
	}
	return micros, nil
}
