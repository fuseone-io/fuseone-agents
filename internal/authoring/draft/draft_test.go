package draft_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/publishable"
	"github.com/fuseone/agents/internal/spec"
)

/*
The draft and the operations that change it (NT-018 1b, contracts 1 and 6).

Every field of the definition has a class, and every change is a typed
operation on one field, validated over the canonical form with the checks a
publication runs: an accepted operation never leaves a draft publication
would refuse for being wrong.
*/

var now = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// The contract's AgentDefinition, read from the source of the generated
// types, so a field added there without a class fails here.
func TestClasses_everyFieldOfTheDefinitionHasOne(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &contract); err != nil {
		t.Fatalf("parse the contract: %v", err)
	}
	fields := contract.Components.Schemas["AgentDefinition"].Properties
	if len(fields) == 0 {
		t.Fatal("found no AgentDefinition properties; the parity has nothing to compare")
	}
	for field := range fields {
		if _, ok := draft.Classes[field]; !ok {
			t.Errorf("AgentDefinition.%s has no class", field)
		}
	}
	for field := range draft.Classes {
		if _, ok := fields[field]; !ok {
			t.Errorf("class for %s, which AgentDefinition does not have", field)
		}
	}
}

// Every field is reachable by an operation of its own class: a field with a
// class and no operation is one nobody can change in the workbench.
func TestClasses_everyFieldHasAnOperationOfItsClass(t *testing.T) {
	t.Parallel()
	covered := map[string]bool{}
	for _, op := range draft.Operations() {
		for _, field := range op.Fields() {
			if draft.Classes[field] != op.Class() {
				t.Errorf("%s edits %s (%s) but is %s", op.Kind(), field, draft.Classes[field], op.Class())
			}
			covered[field] = true
		}
	}
	for field := range draft.Classes {
		if !covered[field] {
			t.Errorf("no operation edits %s", field)
		}
	}
}

// complete is a draft publication would accept.
func complete(t *testing.T) draft.Draft {
	t.Helper()
	s, err := spec.Parse("triage", []byte("---\nid: triage\nname: Triagem\ncompany: acme\narea: cx\n"+
		"provider: openai\nmodel: gpt-test\ntools: [crm.lookup]\nbudget: {micros: 500000, steps: 60}\n---\n\n"+
		"Objetivo\nTriar.\n\nNunca\nPrometer."))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return draft.FromSpec(s)
}

func identityOnly(t *testing.T) draft.Draft {
	t.Helper()
	return draft.New("triage", "Triagem", domain.Scope{Company: "acme", Area: "cx"})
}

type catalogue []domain.ToolEntry

func (c catalogue) Tools(context.Context) ([]domain.ToolEntry, error) { return c, nil }

type configured map[string]bool

func (c configured) Configured(name string, _ domain.Scope) bool { return c[name] }

func validator() draft.Validator {
	return draft.Validator{Checker: publishable.Checker{
		Providers: configured{"openai": true},
		Tools: catalogue{
			{ID: "crm.lookup", Effect: domain.EffectRead, OnSurface: true, Scope: domain.Scope{Company: domain.Installation}},
			{ID: "crm.reply", Effect: domain.EffectWrite, OnSurface: true, Scope: domain.Scope{Company: domain.Installation}},
		},
	}}
}

func apply(t *testing.T, d draft.Draft, op draft.Operation, origin draft.Origin) (draft.Applied, error) {
	t.Helper()
	return validator().Validate(context.Background(), d, op, origin, now)
}

func TestValidate_humanOnlyOperationProposedByTheModel_isInvalid(t *testing.T) {
	t.Parallel()
	for _, op := range []draft.Operation{
		draft.SetIdentity{Name: "Outro", Scope: domain.Scope{Company: "acme", Area: "ops"}},
		draft.SetStandingApprovals{Policy: domain.StandingForbidden},
	} {
		if _, err := apply(t, complete(t), op, draft.FromProposal); !errors.Is(err, draft.ErrHumanOnly) {
			t.Errorf("%s from a proposal = %v, want ErrHumanOnly", op.Kind(), err)
		}
		if _, err := apply(t, complete(t), op, draft.FromControl); err != nil {
			t.Errorf("%s by hand = %v, want it applied", op.Kind(), err)
		}
	}
}

// Contract 3: the instructions change only by an accepted proposal.
func TestValidate_blockOperationByHand_isRefused(t *testing.T) {
	t.Parallel()
	d := complete(t)
	op := draft.ReplaceBlock{ID: d.Blocks[0].ID, Text: "Triar e responder.", Locale: "pt-BR"}
	if _, err := apply(t, d, op, draft.FromControl); !errors.Is(err, draft.ErrConversationOnly) {
		t.Fatalf("a block replaced by hand = %v, want ErrConversationOnly", err)
	}
	got, err := apply(t, d, op, draft.FromProposal)
	if err != nil {
		t.Fatalf("an accepted proposal = %v, want it applied", err)
	}
	if !strings.Contains(got.Draft.Definition().Instructions, "Triar e responder.") {
		t.Errorf("instructions = %q, want the replaced block", got.Draft.Definition().Instructions)
	}
}

// An operation touches only its field; leaving the rest out never clears it.
func TestValidate_anOperation_changesOnlyItsField(t *testing.T) {
	t.Parallel()
	d := complete(t)
	got, err := apply(t, d, draft.SetReplyLocale{Locale: "en-US"}, draft.FromControl)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	before, after := d.Definition(), got.Draft.Definition()
	after.ReplyLocale = before.ReplyLocale
	a, _ := spec.Render(before)
	b, _ := spec.Render(after)
	if string(a) != string(b) {
		t.Fatalf("setting the reply locale changed something else\n%s\n%s", a, b)
	}
	if got.Draft.Definition().ReplyLocale != "en-US" || d.Definition().ReplyLocale != "" {
		t.Error("the locale was not set on the copy, or the original was changed")
	}
}

// What a publication would refuse is refused at the operation, with the
// publication's own words.
func TestValidate_whatPublicationWouldRefuse_isRefusedAtTheOperation(t *testing.T) {
	t.Parallel()
	cases := map[string]draft.Operation{
		"a tool not offered":             draft.AddTool{Tool: "crm.lokup"},
		"a provider not configured":      draft.SetModel{Provider: "anthropic", Model: "x"},
		"a schedule nobody can read":     draft.SetTriggers{Triggers: []spec.Trigger{{Type: "cron", Schedule: "every tuesday"}}},
		"a cache window nobody sells":    draft.SetCachePrefixTTL{TTL: "2h"},
		"a step reaching outside":        draft.AddStep{Step: spec.Step{Name: "Look", Reaches: []domain.ToolID{"crm.reply"}}},
		"removing a tool a step reaches": draft.RemoveTool{Tool: "crm.lookup"},
	}
	for name, op := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := complete(t)
			if name == "removing a tool a step reaches" {
				with, err := apply(t, d, draft.AddStep{Step: spec.Step{Name: "Look", Reaches: []domain.ToolID{"crm.lookup"}}}, draft.FromControl)
				if err != nil {
					t.Fatalf("add the step: %v", err)
				}
				d = with.Draft
			}
			if _, err := apply(t, d, op, draft.FromControl); !errors.Is(err, draft.ErrRefused) {
				t.Fatalf("Validate = %v, want ErrRefused", err)
			}
		})
	}
}

// A draft still being written takes operations; what it lacks is listed.
func TestValidate_aDraftWithOnlyItsIdentity_takesOperationsAndListsWhatIsMissing(t *testing.T) {
	t.Parallel()
	got, err := apply(t, identityOnly(t), draft.AddTool{Tool: "crm.lookup"}, draft.FromControl)
	if err != nil {
		t.Fatalf("Validate = %v, want the tool added to an unfinished draft", err)
	}
	want := []spec.Missing{spec.MissingInstructions, spec.MissingBudget, spec.MissingProvider}
	if !slices.Equal(got.Missing, want) {
		t.Errorf("missing = %v, want %v", got.Missing, want)
	}
}

// Contract 6: the draft has a limit, validated before it is written, and is
// never truncated.
func TestValidate_aDraftOverItsLimit_isRefusedNotTruncated(t *testing.T) {
	t.Parallel()
	d := complete(t)
	huge := strings.Repeat("x", draft.MaxDefinitionBytes)
	if _, err := apply(t, d, draft.InsertBlock{At: 0, Type: blocks.Prose, Text: huge, Locale: "pt-BR"},
		draft.FromProposal); !errors.Is(err, draft.ErrTooLarge) {
		t.Fatalf("Validate = %v, want ErrTooLarge", err)
	}
}

// Opening a published agent and publishing it untouched keeps its version.
func TestFromSpec_untouched_rendersThePublishedBytes(t *testing.T) {
	t.Parallel()
	d := complete(t)
	original, err := spec.Render(func() spec.Spec {
		s, _ := spec.Parse("triage", []byte("---\nid: triage\nname: Triagem\ncompany: acme\narea: cx\n"+
			"provider: openai\nmodel: gpt-test\ntools: [crm.lookup]\nbudget: {micros: 500000, steps: 60}\n---\n\n"+
			"Objetivo\nTriar.\n\nNunca\nPrometer."))
		return s
	}())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	again, err := spec.Render(d.Definition())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(again) != string(original) {
		t.Fatalf("an untouched draft renders other bytes\n%s\n%s", original, again)
	}
}

func TestValidate_operationsOnWhatIsNotThere_areInvalid(t *testing.T) {
	t.Parallel()
	for name, op := range map[string]draft.Operation{
		"a tool twice":            draft.AddTool{Tool: "crm.lookup"},
		"removing an absent tool": draft.RemoveTool{Tool: "crm.reply"},
		"editing an absent step":  draft.EditStep{Name: "Nope", Step: spec.Step{Name: "Nope"}},
		"an unknown block":        draft.RemoveBlock{ID: "b99"},
	} {
		if _, err := apply(t, complete(t), op, draft.FromProposal); !errors.Is(err, draft.ErrInvalidOperation) {
			t.Errorf("%s = %v, want ErrInvalidOperation", name, err)
		}
	}
}

// An operation is applied to a copy: a refused or a later-discarded operation
// leaves the draft it was given exactly as it was.
func TestValidate_neverChangesTheDraftItWasGiven(t *testing.T) {
	t.Parallel()
	with, err := apply(t, complete(t), draft.AddTool{Tool: "crm.reply"}, draft.FromControl)
	if err != nil {
		t.Fatalf("add a second tool: %v", err)
	}
	d := with.Draft
	if _, err := apply(t, d, draft.RemoveTool{Tool: "crm.lookup"}, draft.FromControl); err != nil {
		t.Fatalf("RemoveTool: %v", err)
	}
	if got := d.Definition().Tools; len(got) != 2 || got[0] != "crm.lookup" || got[1] != "crm.reply" {
		t.Fatalf("the original's tools became %v", got)
	}
}

// The workbench's draft always names a whole scope, company and area. The
// generic file parser does not require a company (a file may take it from
// the query); the draft does.
func TestValidate_anIdentityWithoutAWholeScope_isRefused(t *testing.T) {
	t.Parallel()
	for name, scope := range map[string]domain.Scope{
		"no company":                    {Area: "cx"},
		"no area":                       {Company: "acme"},
		"a company publication refuses": {Company: "ACME", Area: "cx"},
		"the installation":              {Company: "*", Area: "cx"},
	} {
		op := draft.SetIdentity{Name: "Triagem", Scope: scope}
		if _, err := apply(t, complete(t), op, draft.FromControl); !errors.Is(err, draft.ErrInvalidOperation) {
			t.Errorf("%s: Validate = %v, want ErrInvalidOperation", name, err)
		}
	}
	noCompany := draft.New("triage", "Triagem", domain.Scope{Area: "cx"})
	if _, err := apply(t, noCompany, draft.AddTool{Tool: "crm.lookup"}, draft.FromControl); !errors.Is(err, draft.ErrRefused) {
		t.Errorf("an operation on a draft with no company = %v, want ErrRefused", err)
	}
}

/*
A validated draft shares nothing with the operation that made it, nor with
the draft it came from. A caller changing the operation's payload after
Validate, or the next draft, would otherwise change a draft already judged,
without judging it again.
*/
func TestValidate_aValidatedDraft_sharesNoNestedSliceWithItsOperationOrItsOrigin(t *testing.T) {
	t.Parallel()
	reaches := []domain.ToolID{"crm.lookup"}
	events := []string{"issues"}
	artifacts := []string{"summary"}
	added, err := apply(t, complete(t), draft.AddStep{Step: spec.Step{Name: "Look", Reaches: reaches}}, draft.FromControl)
	if err != nil {
		t.Fatalf("add_step: %v", err)
	}
	d := added.Draft
	for _, op := range []draft.Operation{
		draft.SetTriggers{Triggers: []spec.Trigger{{Type: "event", Event: "ticket.opened"}}},
		draft.SetEmits{Emits: spec.Emits{{Event: "ticket.triaged", Artifacts: artifacts}}},
	} {
		got, err := apply(t, d, op, draft.FromControl)
		if err != nil {
			t.Fatalf("%s: %v", op.Kind(), err)
		}
		d = got.Draft
	}
	edited, err := apply(t, d, draft.EditStep{Name: "Look", Step: spec.Step{Name: "Look", Reaches: reaches}}, draft.FromControl)
	if err != nil {
		t.Fatalf("edit_step: %v", err)
	}
	webhook, err := apply(t, edited.Draft, draft.SetTriggers{Triggers: []spec.Trigger{
		{Type: "webhook", Path: "gh", From: "github", Events: events},
	}}, draft.FromControl)
	if err != nil {
		t.Fatalf("set_triggers: %v", err)
	}
	reaches[0], events[0], artifacts[0] = "crm.delete", "push", "secrets"

	if r := added.Draft.Spec.Steps[0].Reaches[0]; r != "crm.lookup" {
		t.Errorf("the add_step payload reached the draft: reaches %s", r)
	}
	if r := edited.Draft.Spec.Steps[0].Reaches[0]; r != "crm.lookup" {
		t.Errorf("the edit_step payload reached the draft: reaches %s", r)
	}
	if e := webhook.Draft.Spec.Triggers[0].Events[0]; e != "issues" {
		t.Errorf("the set_triggers payload reached the draft: events %s", e)
	}
	if a := d.Spec.Emits[0].Artifacts[0]; a != "summary" {
		t.Errorf("the set_emits payload reached the draft: artifacts %s", a)
	}

	// And the next draft shares nothing with the one it came from.
	next, err := apply(t, webhook.Draft, draft.SetReplyLocale{Locale: "en-US"}, draft.FromControl)
	if err != nil {
		t.Fatalf("set_reply_locale: %v", err)
	}
	next.Draft.Spec.Triggers[0].Events[0] = "push"
	next.Draft.Spec.Steps[0].Reaches[0] = "crm.delete"
	next.Draft.Spec.Emits[0].Artifacts[0] = "secrets"
	if webhook.Draft.Spec.Triggers[0].Events[0] != "issues" || webhook.Draft.Spec.Steps[0].Reaches[0] != "crm.lookup" {
		t.Error("changing the next draft changed the one it came from")
	}
	if webhook.Draft.Spec.Emits[0].Artifacts[0] != "summary" {
		t.Error("changing the next draft changed the emits of the one it came from")
	}
}

// Every place that names an operation's kind names the same ones: the
// contract's two enums and the proposals table's check, against the
// operations there are. A kind added in one place alone fails here.
func TestKinds_theContractAndTheTableNameEveryOperationAndNoOther(t *testing.T) {
	t.Parallel()
	var want []string
	for _, op := range draft.Operations() {
		want = append(want, op.Kind())
	}
	slices.Sort(want)

	data, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &contract); err != nil {
		t.Fatalf("parse the contract: %v", err)
	}
	for _, schema := range []string{"DraftOperation", "DraftProposal"} {
		got := slices.Sorted(slices.Values(contract.Components.Schemas[schema].Properties["kind"].Enum))
		if !slices.Equal(got, want) {
			t.Errorf("%s.kind enum = %v, want %v", schema, got, want)
		}
	}

	// The newest migration to say the proposal's kinds is the one in force.
	files, err := filepath.Glob("../../ledger/sql/*.sql")
	if err != nil {
		t.Fatalf("list the migrations: %v", err)
	}
	slices.Sort(files)
	kinds := regexp.MustCompile(`(?s)agent_draft_proposals.*?check \(kind in \((.*?)\)\)`)
	var check [][]byte
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if found := kinds.FindSubmatch(sql); found != nil {
			check = found
		}
	}
	if check == nil {
		t.Fatal("no migration checks the proposal's kind")
	}
	got := slices.Sorted(slices.Values(regexp.MustCompile(`'([a-z_]+)'`).FindAllString(string(check[1]), -1)))
	for i := range got {
		got[i] = strings.Trim(got[i], "'")
	}
	if !slices.Equal(got, want) {
		t.Errorf("the proposals table's check = %v, want %v", got, want)
	}
}

/*
A change inside a long block names only the excerpt it changes. Found in
production: a "how to act" block of about 17 thousand characters could only be
rewritten whole, which no turn's output holds, so the conversation changed
steps instead and the behaviour never moved. The edit keeps every other byte of
the block, and what it says does not grow with the block.
*/
func TestValidate_editBlock_inABlockTooLargeToRewrite_changesOnlyItsExcerpt(t *testing.T) {
	t.Parallel()
	before := strings.Repeat("Leia o alerta inteiro antes de agir. ", 250)
	after := strings.Repeat("Feche o caso só com evidência anexada. ", 200)
	s, err := spec.Parse("triage", []byte("---\nid: triage\nname: Triagem\ncompany: acme\narea: cx\n"+
		"provider: openai\nmodel: gpt-test\ntools: [crm.lookup]\nbudget: {micros: 500000, steps: 60}\n---\n\n"+
		"Objetivo\nTriar.\n\nComo agir\n"+before+"Bloqueie o IP 10.0.0.1 sempre. "+after+"\n\nNunca\nPrometer."))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	d := draft.FromSpec(s)
	howToAct := d.Blocks[1]
	if howToAct.Kind != blocks.HowToAct || len(howToAct.Text) < 16000 {
		t.Fatalf("fixture: block %s of %d bytes, want a how-to-act block of production's size", howToAct.Kind, len(howToAct.Text))
	}
	op := draft.EditBlock{ID: howToAct.ID, Old: "Bloqueie o IP 10.0.0.1 sempre.", New: "Bloqueie o IP 10.0.0.1 só com aprovação."}
	if size := len(draft.Body(op)); size > 512 {
		t.Fatalf("the edit says %d bytes, want it bounded by its excerpt, not by its block", size)
	}
	got, err := apply(t, d, op, draft.FromProposal)
	if err != nil {
		t.Fatalf("edit_block = %v, want it applied", err)
	}
	want := strings.TrimSpace(before + "Bloqueie o IP 10.0.0.1 só com aprovação. " + after)
	if text := got.Draft.Blocks[1].Text; text != want {
		t.Fatalf("the block after the edit differs from the excerpt replaced in place (%d bytes, want %d)", len(text), len(want))
	}
	if label := got.Draft.Blocks[1].Label; label != "Como agir" {
		t.Errorf("label = %q, want the block's own: an excerpt does not retitle its block", label)
	}
	if _, err := apply(t, d, op, draft.FromControl); !errors.Is(err, draft.ErrConversationOnly) {
		t.Errorf("an edit by hand = %v, want ErrConversationOnly", err)
	}
}

// The excerpt names one place: absent, repeated or empty, the edit cannot say
// which bytes it changes, and it is invalid rather than a guess.
func TestValidate_editBlock_anExcerptNotThereExactlyOnce_isInvalid(t *testing.T) {
	t.Parallel()
	d := complete(t)
	id := d.Blocks[0].ID // "Triar."
	for name, op := range map[string]draft.EditBlock{
		"absent":        {ID: id, Old: "Responder.", New: "Encerrar."},
		"repeated":      {ID: id, Old: "r", New: "R"},
		"empty":         {ID: id, Old: "", New: "Encerrar."},
		"another block": {ID: "b99", Old: "Triar.", New: "Encerrar."},
	} {
		if _, err := apply(t, d, op, draft.FromProposal); !errors.Is(err, draft.ErrInvalidOperation) {
			t.Errorf("%s = %v, want ErrInvalidOperation", name, err)
		}
	}
}
