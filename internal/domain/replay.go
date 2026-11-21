package domain

/*
A case's world (NT-018 phase 5): the answers an author gave in a rehearsal, in the order the
run asked for them. A battery or an experiment replays a case that keeps one by handing those
answers back, strictly in order, each once. A run that asks for something else has left the
situation the author corrected, and the case is not comparable from there on: reporting it as
held would be the platform inventing evidence.
*/

// StepReplayDiverged records the first call of a replayed case that did not follow its world.
// The engine writes it, once per run; every call after it is answered with nothing.
const StepReplayDiverged StepKind = "replay_diverged"

// WorldEntry is one answer a case's world hands back. Position counts from one, in the order
// the rehearsal asked; the call is known by its tool and the digest of its arguments, and the
// answer by reference and digest, never by its bytes.
type WorldEntry struct {
	Position     int    `json:"position"`
	Tool         ToolID `json:"tool"`
	ArgsDigest   string `json:"args_digest"`
	AnswerRef    string `json:"answer_ref"`
	AnswerDigest string `json:"answer_digest"`
	AnswerBytes  int64  `json:"answer_bytes"`
}

// CaseWorld is the object a case's world reference names. It names its case, so identical
// answers on two cases are two objects and erasing one leaves no gap in the other.
type CaseWorld struct {
	Case    string       `json:"case"`
	Entries []WorldEntry `json:"entries"`
}

// Divergence is how a replayed run left its world. None is a call that followed it.
type Divergence string

const (
	DivergenceNone       Divergence = ""
	DivergenceMissing    Divergence = "missing"
	DivergenceExtra      Divergence = "extra"
	DivergenceRepeated   Divergence = "repeated"
	DivergenceOutOfOrder Divergence = "out_of_order"
)

// ReplayDivergedPayload names the first divergence: the position of the entry the world
// expected next, how the run left it, and the tool it called (the one it never called, for a
// missing one).
type ReplayDivergedPayload struct {
	Position   int        `json:"position"`
	Divergence Divergence `json:"divergence"`
	Tool       ToolID     `json:"tool"`
}

/*
Match answers a call against the world once consumed entries are handed out, in this order
of precedence (D4): the next entry is consumed; an entry already handed out is a repetition;
an entry further on is a call out of order; nothing is a call the world never answered. The
same call both before and after the next entry is a repetition. A call is the same when its
tool and the digest of its arguments are.
*/
func (w CaseWorld) Match(consumed int, tool ToolID, argsDigest string) (WorldEntry, Divergence) {
	same := func(e WorldEntry) bool { return e.Tool == tool && e.ArgsDigest == argsDigest }
	if consumed < len(w.Entries) && same(w.Entries[consumed]) {
		return w.Entries[consumed], DivergenceNone
	}
	for _, e := range w.Entries[:min(consumed, len(w.Entries))] {
		if same(e) {
			return WorldEntry{}, DivergenceRepeated
		}
	}
	for _, e := range w.Entries[min(consumed, len(w.Entries)):] {
		if same(e) {
			return WorldEntry{}, DivergenceOutOfOrder
		}
	}
	return WorldEntry{}, DivergenceExtra
}
