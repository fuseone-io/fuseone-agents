// Package httpapi implements the OpenAPI contract in api/openapi.yaml.
//
// The interface it satisfies is generated from that file, so an endpoint that
// drifts from the contract fails to compile rather than failing in a customer
// integration.
package httpapi

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/trigger"
)

const defaultLimit = 50

// Store is the read side this package needs: the ledger plus enough listing to
// build projections. Declared here, by the consumer.
type Store interface {
	Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error)
	Append(ctx context.Context, s domain.Step) (domain.Step, error)
	Runs(ctx context.Context) ([]domain.RunID, error)
	// Stats, ListRuns and CostRollup answer questions about many runs at once.
	// They are separate from Runs and Read because answering them by folding
	// every run in the ledger does not survive an installation's second year —
	// and an append-only record is guaranteed to have one.
	Stats(ctx context.Context, filter domain.RunFilter) (domain.RunStats, error)
	Throughput(ctx context.Context, filter domain.RunFilter) ([]domain.ThroughputBucket, error)
	Decisions(ctx context.Context, filter domain.RunFilter, limit int) ([]domain.RecordedDecision, error)
	// RunByIdemKey answers what a caller retrying a start already started.
	RunByIdemKey(ctx context.Context, key string) (domain.RunID, error)
	ListRuns(ctx context.Context, filter domain.RunFilter, phase string, limit int) ([]domain.RunSummary, error)
	CostRollup(ctx context.Context, filter domain.RunFilter, groupBy string) ([]domain.CostBucket, error)
	AgentActivity(ctx context.Context, filter domain.RunFilter) ([]domain.AgentActivity, error)
	// SimulationRuns finds the runs one simulation opened. The report is a
	// fold of them, like every other projection here.
	SimulationRuns(ctx context.Context, simulation string) ([]domain.RunID, error)
	// HasSimulation is the gate on an agent leaving Draft (FU-10).
	HasSimulation(ctx context.Context, agent domain.AgentID) (bool, error)
}

// Server implements openapi.StrictServerInterface.
type Server struct {
	store   Store
	version string

	// curator and tools back the administration area. Both are optional: an
	// installation running on the in-memory ledger has no administration
	// store, and the endpoints answer empty rather than failing.
	curator      Curator
	tools        Tools
	integrations Integrations
	agents       Agents
	ceilings     Ceilings
	content      Content
	webhooks     trigger.Webhooks
	audit        audit.Reader
	health       Health
	policies     Policies
	areas        Areas
	authoring    Authoring
	assistants   Assistants
	spend        Spend
	rates        Rates
	pauses       Pauses
	// stops are the switches wider than one agent (PRD FO-06).
	stops Stoppers
	// marks are the budget thresholds each scope has crossed (PRD FO-05).
	marks Marks
	// replays re-derive a run's decisions from what was recorded (PRD AU-07).
	replays Replays
	// composition is the graph of who triggers whom (PRD SE-10).
	composition Composition
	stages      trigger.Stages
	promotions  Promotions
	// cases is where an uploaded simulation set is filed. Optional, like the
	// rest of the authoring area.
	cases Cases
	// identity and signIn administer how people sign in: the stored
	// configuration and the live registry the sign-in routes read from.
	identity Identity
	signIn   SignIn
	// people is the directory of who exists and what each one holds.
	people People
	// regressions is the corpus a future version is checked against, and
	// batteries is where the last run of it against a version is found.
	regressions Regressions
	batteries   LastBattery
	// retention and erasures decide whether content survives.
	retention      Retention
	channels       ChannelAdmin
	companies      CompanyAdmin
	channelListing Lister
	announcer      Announcer
	erasures       Erasures
	// signing is the key exports are sealed with.
	signing   Signing
	publisher Publisher
	// clock is injectable so a run's opening instant is a fact of the request
	// rather than of whichever machine happened to serve it.
	clock Clock
}

// Clock is the time the API stamps its one write with.
type Clock interface {
	Now() time.Time
}

// WithClock replaces the wall clock, which is what makes the opening instant
// assertable in a test.
func (s *Server) WithClock(clock Clock) *Server {
	s.clock = clock
	return s
}

func NewServer(store Store, version string) *Server {
	return &Server{store: store, version: version}
}

// WithAdministration returns the server with the administration area wired.
// Separate from the constructor because the API serves runs whether or not an
// operator can configure anything from it.
func (s *Server) WithAdministration(curator Curator, tools Tools, integrations Integrations) *Server {
	s.curator, s.tools, s.integrations = curator, tools, integrations
	return s
}

// WithAgents wires the registry of published versions.
func (s *Server) WithAgents(agents Agents) *Server {
	s.agents = agents
	return s
}

// WithCeilings wires the budgets configured per scope.
func (s *Server) WithCeilings(ceilings Ceilings) *Server {
	s.ceilings = ceilings
	return s
}

// Health is what the platform observed about the systems it connects to,
// declared here by the consumer. The API only reads: observing is the worker's
// act, because it is the one that connects.
type Health interface {
	All(ctx context.Context) (map[string]domain.IntegrationHealth, error)
}

// WithStages wires how far each agent is trusted to act alone.
func (s *Server) WithStages(stages trigger.Stages) *Server {
	s.stages = stages
	return s
}

// Pauses is whether agents may start, declared here by the consumer.
//
// Wider than the trigger's own reading of it: opening a run asks about one
// agent, and a screen listing twenty asks about all of them at once — which
// is the difference between one query and twenty.
type Pauses interface {
	trigger.Pauses
	Paused(ctx context.Context) (map[domain.AgentID]bool, error)
}

// WithPauses wires whether an agent is allowed to start.
func (s *Server) WithPauses(pauses Pauses) *Server {
	s.pauses = pauses
	return s
}

// WithStops wires the switches wider than one agent (PRD FO-06).
func (s *Server) WithStops(stops Stoppers) *Server {
	s.stops = stops
	return s
}

// WithMarks wires the budget thresholds already crossed.
func (s *Server) WithMarks(marks Marks) *Server {
	s.marks = marks
	return s
}

// WithReplays wires what a faithful replay needs: the policy set each decision
// was made under, and the pack of the version that ran.
func (s *Server) WithReplays(replays Replays) *Server {
	s.replays = replays
	return s
}

// WithComposition wires the graph of who triggers whom.
func (s *Server) WithComposition(composition Composition) *Server {
	s.composition = composition
	return s
}

// WithHealth wires the observations beside the configuration.
func (s *Server) WithHealth(health Health) *Server {
	s.health = health
	return s
}

var _ openapi.StrictServerInterface = (*Server)(nil)

func (s *Server) Health(context.Context, openapi.HealthRequestObject) (openapi.HealthResponseObject, error) {
	return openapi.Health200JSONResponse{Status: openapi.Ok, Version: s.version}, nil
}
