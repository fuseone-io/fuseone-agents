package conversation

import "github.com/fuseone/agents/internal/authoring/drafts"

// Fate is what became of a proposal: accepted, rejected, still waiting, stale, or invalid.
type Fate string

const (
	FateAccepted Fate = "accepted"
	FateRejected Fate = "rejected"
	FateWaiting  Fate = "waiting"
	FateInvalid  Fate = "invalid"
	// FateStale is a valid proposal nobody decided that the draft, as it is, would no
	// longer accept: something other than its own turn changed it since (#412).
	FateStale Fate = "stale"
)

// Outcome is one earlier proposal as the model is told of it: its kind and its fate. Never
// its operation or reason, which the model wrote and the transcript keeps.
type Outcome struct {
	Kind string
	Fate Fate
}

// fateWords is each fate as the prompt says it, in the locale's words. The words say
// whether the proposal is in the draft, which is what the model must not assume.
var fateWords = map[string]map[Fate]string{
	"pt-BR": {
		FateAccepted: "aceita, já aplicada ao rascunho",
		FateRejected: "recusada pela pessoa, não aplicada",
		FateWaiting:  "ainda esperando a pessoa, não aplicada",
		FateInvalid:  "inválida, nunca aplicada",
		FateStale:    "obsoleta, o rascunho mudou e ela não pode mais ser aceita; proponha de novo se ainda fizer sentido",
	},
	"en-US": {
		FateAccepted: "accepted, applied to the draft",
		FateRejected: "rejected by the person, not applied",
		FateWaiting:  "still waiting for the person, not applied",
		FateInvalid:  "invalid, never applied",
		FateStale:    "stale, the draft changed and it can no longer be accepted; propose it again if it still makes sense",
	},
}

// Outcomes is the thread's proposals by the turn that made them, each with its fate, in the
// thread's order. A proposal of no turn is not in any window, and is left out.
func Outcomes(thread []drafts.Proposal) map[string][]Outcome {
	out := map[string][]Outcome{}
	for _, p := range thread {
		if p.Turn == "" {
			continue
		}
		out[p.Turn] = append(out[p.Turn], Outcome{Kind: p.Kind, Fate: fateOf(p)})
	}
	return out
}

func fateOf(p drafts.Proposal) Fate {
	switch {
	case !p.Valid:
		return FateInvalid
	case p.Decision == "accepted":
		return FateAccepted
	case p.Decision == "rejected":
		return FateRejected
	case !p.Current:
		return FateStale
	}
	return FateWaiting
}
