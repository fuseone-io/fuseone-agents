package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNotPublished means no such agent version is on record.
var ErrNotPublished = errors.New("spec: no such published version")

// Registry is where published specifications live.
//
// The Store holds what a process loaded; this holds what the installation has
// published. The distinction matters because a run is pinned to a version: the
// file on disk can change, and the version that ran must not.
type Registry struct {
	pool *pgxpool.Pool
}

func NewRegistry(pool *pgxpool.Pool) *Registry {
	return &Registry{pool: pool}
}

// Publish records a version, or does nothing if it is already there.
//
// Insert-or-nothing rather than upsert: the version is the digest of the
// content, so the same version is the same text. A conflict means somebody
// published what was already published, which is not an error and must not
// overwrite the original's authorship or date.
func (r *Registry) Publish(ctx context.Context, s Spec, by domain.UserID, company domain.CompanyID) error {
	if company == "" || company == domain.Installation {
		return fmt.Errorf("spec: publish %s@%s: company %q is not a publishable company", s.ID, s.Version, company)
	}
	if s.Company != "" && s.Company != company {
		return fmt.Errorf("spec: publish %s@%s: definition company %q does not match authorised company %q",
			s.ID, s.Version, s.Company, company)
	}

	budget, err := json.Marshal(s.Budget)
	if err != nil {
		return fmt.Errorf("spec: encode budget: %w", err)
	}
	triggers, err := json.Marshal(triggersOf(s))
	if err != nil {
		return fmt.Errorf("spec: encode triggers: %w", err)
	}

	tools := make([]string, 0, len(s.Tools))
	for _, t := range s.Tools {
		tools = append(tools, string(t))
	}
	// Never nil: a nil slice reaches Postgres as null, and the column says not
	// null because "declares no events" is the empty list rather than unknown.
	emits := s.Emits
	if emits == nil {
		emits = Emits{}
	}
	encodedEmits, err := json.Marshal(emits)
	if err != nil {
		return fmt.Errorf("spec: encode emits: %w", err)
	}
	// The stages, for the same reason: an agent that declares none is the
	// empty list, which is one envelope holding the whole pack — a different
	// thing from a step that reaches nothing.
	declared := s.Steps
	if declared == nil {
		declared = []Step{}
	}
	steps, err := json.Marshal(declared)
	if err != nil {
		return fmt.Errorf("spec: encode steps: %w", err)
	}
	learning, err := json.Marshal(s.MemoryLearning.Normalize())
	if err != nil {
		return fmt.Errorf("spec: encode memory learning: %w", err)
	}
	approvals, err := json.Marshal(s.Approvals.Normalize())
	if err != nil {
		return fmt.Errorf("spec: encode approvals: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		insert into agent_specs (
			agent_id, version_id, company_id, area_id, name,
			provider, model, effort, tools, budget, triggers,
			instructions, source, published_by, emits, steps, memory_learning,
			approvals
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		on conflict (agent_id, version_id) do nothing`,
		string(s.ID), string(s.Version), string(company), string(s.Area), s.Name,
		s.Provider, s.Model, s.Effort, tools, budget, triggers,
		s.Instructions, s.Source, string(by), encodedEmits, steps, learning,
		approvals)
	if err != nil {
		return fmt.Errorf("spec: publish %s@%s: %w", s.ID, s.Version, err)
	}
	return nil
}

// Get returns one published version, exactly as it was published.
func (r *Registry) Get(ctx context.Context, agent domain.AgentID, version domain.VersionID) (Spec, error) {
	var (
		s                Spec
		tools            []string
		budget, triggers []byte
		emits, steps     []byte
		learning         []byte
		approvals        []byte
		company          string
	)
	err := r.pool.QueryRow(ctx, `
		select agent_id, version_id, company_id, area_id, name,
		       provider, model, effort, tools, budget, triggers, instructions,
		       source, emits, steps, memory_learning, approvals
		from agent_specs where agent_id = $1 and version_id = $2`,
		string(agent), string(version),
	).Scan(&s.ID, &s.Version, &company, &s.Area, &s.Name,
		&s.Provider, &s.Model, &s.Effort, &tools, &budget, &triggers,
		&s.Instructions, &s.Source, &emits, &steps, &learning, &approvals)

	if errors.Is(err, pgx.ErrNoRows) {
		return Spec{}, fmt.Errorf("%w: %s@%s", ErrNotPublished, agent, version)
	}
	if err != nil {
		return Spec{}, fmt.Errorf("spec: read %s@%s: %w", agent, version, err)
	}

	for _, t := range tools {
		s.Tools = append(s.Tools, domain.ToolID(t))
	}
	s.Company = domain.CompanyID(company)
	if err := json.Unmarshal(budget, &s.Budget); err != nil {
		return Spec{}, fmt.Errorf("spec: decode budget: %w", err)
	}
	var stored []domain.AgentTrigger
	if err := json.Unmarshal(triggers, &stored); err != nil {
		return Spec{}, fmt.Errorf("spec: decode triggers: %w", err)
	}
	for _, t := range stored {
		s.Triggers = append(s.Triggers, Trigger{Type: t.Type, Schedule: t.Schedule, Path: t.Path, Event: t.Event})
	}
	if err := json.Unmarshal(emits, &s.Emits); err != nil {
		return Spec{}, fmt.Errorf("spec: decode emits: %w", err)
	}
	if err := json.Unmarshal(steps, &s.Steps); err != nil {
		return Spec{}, fmt.Errorf("spec: decode steps: %w", err)
	}
	if err := json.Unmarshal(learning, &s.MemoryLearning); err != nil {
		return Spec{}, fmt.Errorf("spec: decode memory learning: %w", err)
	}
	s.MemoryLearning = s.MemoryLearning.Normalize()
	// The same read the reporter uses. Two decoders for one column is how a
	// shape one of them refuses is obeyed by the other.
	if s.Approvals, err = decodeApprovals(approvals); err != nil {
		return Spec{}, err
	}
	return s, nil
}

// Declarations is the half of a published version a summary leaves out.
//
// One answer rather than three, because they are dropped for the same reason:
// what a read does not return, an editor cannot put back, and publishing again
// deletes it. Separate calls are separate chances to forget one — which is
// exactly how the approval policy was lost between a version and its next
// publication.

// Declared reads the parts of a published version a summary leaves out.
//
// Its own read rather than fields on the summary: a listing of twenty agents
// would carry twenty processes nobody asked to see, which is the same reason
// the instructions are read one version at a time. Both in one answer,
// because what a read omits an editor cannot put back — and publishing again
// deletes it.
type Declarations struct {
	Steps     []Step
	Emits     Emits
	Approvals domain.ApprovalPolicy
}

func (r *Registry) Declared(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (Declarations, error) {
	var raw, emits, approvals []byte
	err := r.pool.QueryRow(ctx, `
		select steps, emits, approvals from agent_specs
		where agent_id = $1 and version_id = $2`,
		string(agent), string(version)).Scan(&raw, &emits, &approvals)

	if errors.Is(err, pgx.ErrNoRows) {
		return Declarations{}, nil
	}
	if err != nil {
		return Declarations{}, fmt.Errorf("spec: read declarations of %s@%s: %w", agent, version, err)
	}

	var out Declarations
	if err := json.Unmarshal(raw, &out.Steps); err != nil {
		return Declarations{}, fmt.Errorf("spec: decode steps: %w", err)
	}
	if err := json.Unmarshal(emits, &out.Emits); err != nil {
		return Declarations{}, fmt.Errorf("spec: decode emits: %w", err)
	}
	if out.Approvals, err = decodeApprovals(approvals); err != nil {
		return Declarations{}, err
	}
	return out, nil
}
