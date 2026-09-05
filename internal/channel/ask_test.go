package channel_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel"
)

/*
Which agent a message is for.

The mention names it, which is why a channel trigger needs no configuration:
what governs whether it may run here is the intersection of two facts that
already exist — the conversation maps to a scope, and the agent lives in it.
*/

var startable = []channel.Startable{
	{ID: "triagem", Name: "Triagem de chamados"},
	{ID: "reconciliation", Name: "Reconciliation"},
}

func TestRead_theMentionNamesIt(t *testing.T) {
	t.Parallel()

	got, err := channel.Read("<@U07BOT> triagem esse chamado é sobre boleto", startable, "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Agent != "triagem" {
		t.Errorf("agent = %q, want triagem", got.Agent)
	}
}

// An author names their agent in their own language and an operator knows it
// by its id. Both are what somebody would type, and case is not something a
// person is quoting when they type into a chat.
func TestRead_matchesTheIdAndTheNameHoweverManyWordsItHas(t *testing.T) {
	t.Parallel()

	for _, said := range []string{
		"<@U07BOT> triagem esse chamado",
		"<@U07BOT> Triagem esse chamado",
		// The name is several words, which most names are. Matching the first
		// word alone would support the agents somebody happened to name in one
		// and silently never the rest.
		"<@U07BOT> Triagem de chamados: esse chamado",
	} {
		got, err := channel.Read(said, startable, "")
		if err != nil || got.Agent != "triagem" {
			t.Errorf("%q -> %q, %v", said, got.Agent, err)
		}
	}
}

// `triagemzinho` is not `triagem`. Read as one it would send an ask to an
// agent nobody addressed.
func TestRead_aNameThatMerelyStartsTheSame_isNotThatAgent(t *testing.T) {
	t.Parallel()

	if _, err := channel.Read("<@U07BOT> triagemzinho faz algo", startable, ""); err == nil {
		t.Error("a longer word resolved to the agent whose name it begins with")
	}
}

/*
Nothing named is a refusal, not a choice.

With one startable agent it would look like the only possible reading, and it
would still be the platform deciding — the day a second agent is added the same
sentence means something else and nobody changed it. Every other part of this
design refuses to infer: the exception is not read out of prose and the scope
is not read out of the text.
*/
func TestRead_namingNoAgent_isRefusedRatherThanGuessed(t *testing.T) {
	t.Parallel()

	_, err := channel.Read("<@U07BOT>", []channel.Startable{startable[0]}, "")
	if !errors.Is(err, channel.ErrNoAgentNamed) {
		t.Fatalf("err = %v, want ErrNoAgentNamed even with one candidate", err)
	}
}

// The refusal is the only teaching surface a channel has: nobody reads
// documentation before typing in a chat, and a "no" that does not say what
// would have worked is one somebody gives up after.
func TestRead_aRefusal_saysWhatWouldHaveWorked(t *testing.T) {
	t.Parallel()

	_, err := channel.Read("<@U07BOT> vendas alguma coisa", startable, "")
	if !errors.Is(err, channel.ErrNotStartable) {
		t.Fatalf("err = %v, want ErrNotStartable", err)
	}
	if !strings.Contains(err.Error(), "triagem") {
		t.Errorf("refusal = %q, want it to name what is startable", err)
	}
}

/*
An agent outside this conversation's scope and one that never declared
willingness are refused identically.

Distinguishing them would tell somebody which agents exist in a scope they
cannot reach, from a channel anybody in it can type into.
*/
func TestRead_anAgentNotOnTheList_saysNothingAboutWhy(t *testing.T) {
	t.Parallel()

	_, quiet := channel.Read("<@U07BOT> folha algo", startable, "")
	_, absent := channel.Read("<@U07BOT> naoexiste algo", startable, "")

	if quiet.Error() == absent.Error() {
		return
	}
	// Both must read the same but for the word the person typed.
	if strings.Replace(quiet.Error(), "folha", "X", 1) !=
		strings.Replace(absent.Error(), "naoexiste", "X", 1) {
		t.Errorf("the two refusals differ:\n%v\n%v", quiet, absent)
	}
}

// What the run records as having been said is the person's own words. The
// mention is the envelope and the agent's name is addressing, and neither is
// part of the ask.
func TestRead_dropsTheEnvelopeAndKeepsTheWords(t *testing.T) {
	t.Parallel()

	got, err := channel.Read("<@U07BOT> Triagem de chamados: esse chamado é sobre boleto",
		startable, "")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Text != "esse chamado é sobre boleto" {
		t.Errorf("ask = %q, want the person's own words", got.Text)
	}
}

/*
A conversation an administrator bound to an agent no longer needs the name.

This is not the guess the refusals above exist to prevent. Nothing is read out
of the text and nothing is picked for being the only candidate: somebody wrote
that agent down against that conversation, and the whole sentence is the ask
because none of it was ever a name.
*/
func TestRead_namingNoAgentWhereOneIsBound_startsTheBoundAgent(t *testing.T) {
	t.Parallel()

	got, err := channel.Read("<@U07BOT> esse chamado é sobre boleto", startable, "triagem")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Agent != "triagem" {
		t.Errorf("agent = %q, want the bound agent", got.Agent)
	}
	if got.Text != "esse chamado é sobre boleto" {
		t.Errorf("ask = %q, want the whole sentence", got.Text)
	}
}

// A bare mention resolves to the bound agent and carries no words. Whether an
// ask with no words is worth a run is not this function's question — it has no
// idea what came before the message — and the consumer decides it.
func TestRead_aBareMentionWhereAnAgentIsBound_resolvesToIt(t *testing.T) {
	t.Parallel()

	got, err := channel.Read("<@U07BOT>", startable, "triagem")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Agent != "triagem" || got.Text != "" {
		t.Errorf("ask = %+v, want the bound agent and no words", got)
	}
}

// The binding is a default and not a redirection. What somebody addressed by
// name is what they meant, and silently sending it elsewhere would make the
// name they typed a decoration.
func TestRead_namingAnAgentWhereAnotherIsBound_honoursTheName(t *testing.T) {
	t.Parallel()

	got, err := channel.Read("<@U07BOT> reconciliation fecha o mês", startable, "triagem")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Agent != "reconciliation" || got.Text != "fecha o mês" {
		t.Errorf("ask = %+v, want the agent that was named", got)
	}
}
