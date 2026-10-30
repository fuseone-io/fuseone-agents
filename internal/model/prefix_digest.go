package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/fuseone/agents/internal/domain"
)

// digestLen keeps a fingerprint short enough to read in a query result and
// long enough that two different blocks of one agent do not collide.
const digestLen = 16

// anthropicPrefixDigests fingerprints the tools and each system block exactly
// as the SDK encodes them, so the digest moves with any byte the cache sees.
func anthropicPrefixDigests(params anthropic.MessageNewParams) *domain.PromptDigests {
	out := &domain.PromptDigests{Tools: digestOf(params.Tools)}
	for _, block := range params.System {
		out.System = append(out.System, digestOf(block))
	}
	return out
}

// openAIPrefixDigests does the same for the chat wire, where the system
// prompt is the leading system message.
func openAIPrefixDigests(body chatRequest) *domain.PromptDigests {
	out := &domain.PromptDigests{Tools: digestOf(body.Tools)}
	for _, msg := range body.Messages {
		if msg.Role != "system" {
			break
		}
		out.System = append(out.System, digestOf(msg))
	}
	return out
}

// digestOf is empty when the value cannot be encoded: the request would fail
// on the same encoding, and an absent digest reads as "not measured".
func digestOf(v any) string { // any: SDK params and chat messages share no type
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:digestLen]
}
