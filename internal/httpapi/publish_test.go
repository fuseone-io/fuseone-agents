package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
)

// Editing an agent is authoring its next version. The screen has to make that
// true rather than only say it.

type publisher struct {
	published []spec.Spec
	paused    map[domain.AgentID]bool
	ensured   []domain.AgentID
}

func newPublisher() *publisher {
	return &publisher{paused: map[domain.AgentID]bool{}}
}

func (p *publisher) Publish(_ context.Context, s spec.Spec, _ domain.UserID, _ domain.CompanyID) error {
	p.published = append(p.published, s)
	return nil
}

func (p *publisher) EnsurePaused(_ context.Context, agent domain.AgentID, _ domain.UserID) error {
	p.ensured = append(p.ensured, agent)
	if _, decided := p.paused[agent]; !decided {
		p.paused[agent] = true
	}
	return nil
}

func (p *publisher) SetPaused(_ context.Context, agent domain.AgentID, paused bool, _ domain.UserID) error {
	p.paused[agent] = paused
	return nil
}

func (p *publisher) IsPaused(_ context.Context, agent domain.AgentID) (bool, error) {
	stopped, decided := p.paused[agent]
	return !decided || stopped, nil
}

func definition(over func(*openapi.AgentDefinition)) *openapi.PublishAgentJSONRequestBody {
	body := openapi.AgentDefinition{
		Name: "Atendimento", Area: "cx", Provider: "openai", Model: "gpt-test",
		Instructions: "Atenda o chamado e responda.",
		Tools:        &[]string{"crm.lookup"},
		// A definition with no ceiling is one the parser refuses, and this
		// fixture has to be a definition a file could hold.
		Budget: &openapi.Budget{Micros: ptr(int64(500_000)), Steps: ptr(int64(60))},
	}
	if over != nil {
		over(&body)
	}
	return &body
}

func publishServer(t *testing.T, p *publisher) *Server {
	t.Helper()
	return NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).WithPublisher(p)
}

func TestPublishAgent_writesTheVersionAndRecordsItPaused(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	resp, err := publishServer(t, pub).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	got, ok := resp.(openapi.PublishAgent200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the published version", resp)
	}
	if got.VersionId == "" {
		t.Error("no version came back")
	}
	// Authoring never starts anything.
	if !got.Paused {
		t.Error("a newly published agent reports itself running")
	}
	if len(pub.published) != 1 || pub.published[0].ID != "triage" {
		t.Errorf("published = %+v, want the agent", pub.published)
	}
}

func TestPublishAgent_eventContextSurvivesTheConsolePath(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	_, err := publishServer(t, pub).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(func(d *openapi.AgentDefinition) {
			d.Emits = &[]openapi.AgentEvent{{
				Event: "incident.triaged", Context: ptr("incident"),
				Artifacts: &[]string{"triage_summary", "suspected_cause"},
			}}
		})},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	if len(pub.published) != 1 || len(pub.published[0].Emits) != 1 {
		t.Fatalf("published emits = %+v, want the event declaration", pub.published)
	}
	got := pub.published[0].Emits[0]
	if got.Event != "incident.triaged" || got.Context != "incident" ||
		len(got.Artifacts) != 2 || got.Artifacts[1] != "suspected_cause" {
		t.Errorf("published event = %+v, want the context-carrying declaration", got)
	}
}

func TestPublishAgent_returnsTheFileItPublished(t *testing.T) {
	t.Parallel()

	// An installation that keeps its agents in git commits this, and one that
	// writes files can open them here. Neither direction is a migration.
	resp, err := publishServer(t, newPublisher()).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	got := resp.(openapi.PublishAgent200JSONResponse)

	if got.Definition == nil || *got.Definition == "" {
		t.Fatal("no definition came back")
	}
	// It has to be the real thing: parseable, and the same version.
	parsed, err := spec.Parse("test", []byte(*got.Definition))
	if err != nil {
		t.Fatalf("what came back does not parse: %v", err)
	}
	if string(parsed.Version) != got.VersionId {
		t.Errorf("the file is version %s and the response says %s", parsed.Version, got.VersionId)
	}
}

func TestPublishAgent_theSameDefinitionTwice_makesNoSecondVersion(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	server := publishServer(t, pub)

	first, err := server.PublishAgent(inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	second, err := server.PublishAgent(inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)})
	if err != nil {
		t.Fatalf("PublishAgent again: %v", err)
	}

	// The version is the digest of the bytes. Two saves of the same words are
	// one version, or a run pinned to either would be pinned to a coin toss.
	a := first.(openapi.PublishAgent200JSONResponse)
	b := second.(openapi.PublishAgent200JSONResponse)
	if a.VersionId != b.VersionId {
		t.Errorf("versions %s and %s for identical text", a.VersionId, b.VersionId)
	}
}

func TestPublishAgent_aDefinitionAFileWouldRefuse_isRefusedHere(t *testing.T) {
	t.Parallel()

	// Validation lives in the parser. A console that skipped it would accept
	// definitions no file could, and the two would stop being one format.
	resp, err := publishServer(t, newPublisher()).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{
			AgentId: "triage",
			Body:    definition(func(d *openapi.AgentDefinition) { d.Instructions = "" }),
		},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, refused := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 400", resp)
	}
}

func TestPublishAgent_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	resp, err := publishServer(t, newPublisher()).PublishAgent(
		inArea("cx", domain.RoleApprover),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, refused := resp.(openapi.PublishAgent403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestPublishAgent_republishingAStartedAgent_leavesItStarted(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	pub.paused["triage"] = false

	resp, err := publishServer(t, pub).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	// Editing a prompt is not a reason to stop an agent somebody deliberately
	// started, and finding out at three in the morning would be worse.
	if resp.(openapi.PublishAgent200JSONResponse).Paused {
		t.Error("republishing stopped a running agent")
	}
}

func TestSetAgentPaused_needsTheAuthorityToCauseRuns(t *testing.T) {
	t.Parallel()

	// Starting an agent is causing every run it will make, so it needs the
	// authority to cause runs. An auditor reads everything this agent ever
	// did and must not be able to make it do more.
	resp, err := publishServer(t, newPublisher()).SetAgentPaused(
		inArea("cx", domain.RoleAuditor),
		openapi.SetAgentPausedRequestObject{
			AgentId: "triage",
			Body:    &openapi.SetAgentPausedJSONRequestBody{Paused: false},
		},
	)
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, refused := resp.(openapi.SetAgentPaused403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

/*
Republishing an agent must not quietly drop its steps.

The steps are the half of a definition the Gate obeys: `reaches` is what a run
may call while it sits at one, so the capability pack is the ceiling and the
step is the actual permission. A console that rendered a specification without
them would widen every agent it touched back to the whole pack, silently, on
an edit somebody made for another reason.
*/
func TestPublishAgent_withDeclaredSteps_keepsThem(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	body := definition(nil)
	body.Steps = &[]openapi.AgentStep{
		{Name: "Encontrar o cliente", Reaches: ptr([]string{"crm.lookup"}),
			StopsWhen: ptr("não encontrar o cliente")},
		{Name: "Responder", Reaches: ptr([]string{"crm.reply"})},
	}

	if _, err := publishServer(t, pub).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: body},
	); err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	if len(pub.published) != 1 {
		t.Fatalf("published = %d", len(pub.published))
	}
	steps := pub.published[0].Steps
	if len(steps) != 2 {
		t.Fatalf("steps = %+v, want both", steps)
	}
	if steps[0].Name != "Encontrar o cliente" || len(steps[0].Reaches) != 1 {
		t.Errorf("first step = %+v", steps[0])
	}
	// The exception belongs to the step, which is what lets a correction be
	// localised to where it went wrong (FU-13).
	if steps[0].StopsWhen != "não encontrar o cliente" {
		t.Errorf("stops_when = %q, want the author's own words", steps[0].StopsWhen)
	}
}

/*
A trigger declared in a version is a trigger the platform reaches.

The console let somebody choose a schedule, stored it in the version, printed
it back on the screen and nothing ever fired: schedules were reconciled from
specification files at worker start-up, so an agent authored here was never
reachable by the clock at all. The screen kept a promise the platform had no
way of keeping, and the only symptom was an agent that never ran.

Reconciled where the version is written, so both ways of publishing —
committing a file and pressing the button — end at the same table.
*/
func TestPublishAgent_declaresACronTrigger_scheduleIsReconciled(t *testing.T) {
	t.Parallel()
	moments := &schedules{}

	if _, err := publishServer(t, newPublisher()).WithSchedules(moments).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{
			AgentId: "triage",
			Body: definition(func(d *openapi.AgentDefinition) {
				d.Triggers = &[]openapi.AgentTrigger{
					{Type: "cron", Schedule: ptr("0 7 * * 1-5")},
				}
			}),
		},
	); err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	if got := moments.synced["triage"]; len(got) != 1 || got[0] != "0 7 * * 1-5" {
		t.Errorf("synced = %v, want the schedule the version declares", got)
	}
}

// A version that withdrew its schedule stops firing it, which is the same call
// with an empty list rather than a second one.
func TestPublishAgent_declaresNoTrigger_reconcilesToNothing(t *testing.T) {
	t.Parallel()
	moments := &schedules{}

	if _, err := publishServer(t, newPublisher()).WithSchedules(moments).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(nil)},
	); err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	got, called := moments.synced["triage"]
	if !called || len(got) != 0 {
		t.Errorf("synced = %v (called %v), want an empty list", got, called)
	}
}

// Refused before the version is written, because a schedule nobody can parse
// would otherwise be published, reported as saved, and never fire.
func TestPublishAgent_scheduleNobodyCanParse_isRefusedBeforePublishing(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	resp, err := publishServer(t, pub).WithSchedules(&schedules{}).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{
			AgentId: "triage",
			Body: definition(func(d *openapi.AgentDefinition) {
				d.Triggers = &[]openapi.AgentTrigger{
					{Type: "cron", Schedule: ptr("toda segunda de manhã")},
				}
			}),
		},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, refused := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if len(pub.published) != 0 {
		t.Error("a version was written for a schedule that cannot fire")
	}
}

type schedules struct{ synced map[domain.AgentID][]string }

func (s *schedules) Sync(
	_ context.Context, agent domain.AgentID, list []string, _ time.Time,
) error {
	if s.synced == nil {
		s.synced = map[domain.AgentID][]string{}
	}
	s.synced[agent] = list
	return nil
}
