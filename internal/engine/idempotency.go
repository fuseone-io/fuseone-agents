package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

const readPollSeparator = ":poll:"

// idempotencyKey identifies a call by what it does, not by where it sits.
//
// The step sequence is deliberately excluded. A resumed run re-plans and lands
// at a different sequence number, so a position-dependent key would look new
// on every retry and duplicate a write whose outcome is already recorded.
//
// The key is recorded for every call. Writes use it as exactly-once safety;
// completed reads use it for result-cache identity and no-progress detection
// while remaining free to poll a changing source. A read whose result never
// reached the ledger is still blocked on resume because its outcome is unknown.
func idempotencyKey(runID domain.RunID, tool domain.ToolID, args []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|", runID, tool)
	_, _ = h.Write(domain.CanonicalCallArguments(args))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func executionIdempotencyKey(
	state State, effect domain.Effect, tool domain.ToolID, base string,
) string {
	if effect != domain.EffectRead || tool == domain.ToolMemoryFind {
		return base
	}
	completed := state.completedReadCount(base)
	if completed == 0 {
		return base
	}
	return fmt.Sprintf("%s%s%d", base, readPollSeparator, completed+1)
}

func readIdempotencyBase(key string) string {
	if at := strings.LastIndex(key, readPollSeparator); at >= 0 {
		return key[:at]
	}
	return key
}

func duplicateWithinRun(state State, effect domain.Effect, tool domain.ToolID, idemKey string) bool {
	if !state.AlreadyExecuted(idemKey) {
		return false
	}
	// Writes and irreversible effects remain exactly-once. The platform-owned
	// memory lookup also refuses an equivalent retry because it is a snapshot
	// of run memory, not a status poll. Other completed reads may legitimately
	// observe a changing world; orphaned reads stay blocked because their
	// external outcome and cost are unknown.
	return effect != domain.EffectRead || tool == domain.ToolMemoryFind || !state.CompletedCall(idemKey)
}
