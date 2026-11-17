package draft

// Class is who may change a field of the definition (NT-018, contract 1).
type Class string

const (
	// Human fields are changed by a person's own act and never proposed by
	// the model: they decide where the agent runs and what authority already
	// granted may release.
	Human Class = "human"
	// Conversation fields change only through a proposal a person accepted.
	Conversation Class = "conversation"
	// Shared fields change through the conversation or the controls, by the
	// same operations.
	Shared Class = "shared"
)

// Classes is every field of AgentDefinition, by its name in the contract. The
// identifier is not one of them: it is the path, immutable once the draft is
// born. A parity test reads the contract and fails on a field missing here.
var Classes = map[string]Class{
	"name":              Human,
	"company":           Human,
	"area":              Human,
	"standingApprovals": Human,
	"instructions":      Conversation,
	"provider":          Shared,
	"model":             Shared,
	"effort":            Shared,
	"fallback":          Shared,
	"cachePrefixTtl":    Shared,
	"replyLocale":       Shared,
	"tools":             Shared,
	"budget":            Shared,
	"triggers":          Shared,
	"memoryLearning":    Shared,
	"approvals":         Shared,
	"emits":             Shared,
	"steps":             Shared,
}
