package engine

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
)

const (
	toolResultCompactAfter     = 32 << 10
	toolResultHeadBytes        = 16 << 10
	toolResultTailBytes        = 8 << 10
	toolResultTranscriptBudget = 64 << 10
	// receiptOmittedWidth makes every int64 rendering the same width. The
	// receipt computes omitted bytes from a zero-valued probe; without a fixed
	// width, writing the real number would change the receipt length and make
	// that count wrong.
	receiptOmittedWidth = 20
)

func compactToolResultForTranscript(tool domain.ToolID, content []byte) []byte {
	var ignored int64
	return compactToolResult(tool, content, &ignored)
}

func compactToolResult(tool domain.ToolID, content []byte, elided *int64) []byte {
	if len(content) <= toolResultCompactAfter || !compactableLargeToolResult(tool) {
		return content
	}
	head := utf8Prefix(content, toolResultHeadBytes)
	tail := utf8Suffix(content, toolResultTailBytes)
	var b strings.Builder
	fmt.Fprintf(&b, "FuseOne compacted this %s result before sending it back to the model.\n", tool)
	fmt.Fprintf(&b, "Stored result: %d bytes, digest %s.\n", len(content), digest(content))
	b.WriteString("Only the beginning and end are shown here. Do not treat the omitted middle as absent; call a narrower query if this is not enough.\n\n")
	fmt.Fprintf(&b, "--- first %d bytes ---\n%s\n\n", len(head), head)
	removed := max(0, len(content)-len(head)-len(tail))
	*elided += int64(removed)
	fmt.Fprintf(&b, "--- omitted %d bytes ---\n\n", removed)
	fmt.Fprintf(&b, "--- last %d bytes ---\n%s", len(tail), tail)
	return []byte(b.String())
}

// boundToolResultTranscript keeps result bytes bounded in stable generations.
// A generation remains byte-for-byte unchanged while new results fit. When it
// crosses the budget, the whole generation becomes receipts at once; rebuilding
// a later turn therefore does not move the receipt boundary one result at a
// time and invalidate the provider's prefix cache on every call.
//
// The stored copy remains subject to the installation content limit, and
// Elided is set from the original result so run-spend diagnostics attribute
// the saving to the tool that produced it.
func boundToolResultTranscript(turns []Turn) {
	type candidate struct {
		index   int
		receipt []byte
	}
	var candidates []candidate
	sent := 0
	generationStart := 0
	for i := range turns {
		if turns[i].Kind != TurnToolResult || len(turns[i].Content) == 0 {
			continue
		}
		receipt := toolResultBudgetReceipt(turns[i])
		if len(receipt) >= len(turns[i].Content) {
			receipt = turns[i].Content
		}
		candidates = append(candidates, candidate{index: i, receipt: receipt})
		sent += len(turns[i].Content)
		if sent <= toolResultTranscriptBudget {
			continue
		}

		// Compact the completed generation but keep the newest result whole: it
		// is the evidence the model just asked for. This leaves headroom for
		// later turns while making each earlier generation immutable.
		generationEnd := len(candidates) - 1
		if generationStart == generationEnd {
			// One result alone can exceed the aggregate ceiling when its tool has
			// no safe per-result compactor. A receipt is the only bounded form.
			generationEnd++
		}
		for j := generationStart; j < generationEnd; j++ {
			candidate := candidates[j]
			turn := &turns[candidate.index]
			sent -= len(turn.Content)
			turn.Content = candidate.receipt
			turn.Elided = max(int64(0), turn.OriginalBytes-int64(len(candidate.receipt)))
			sent += len(candidate.receipt)
		}
		generationStart = generationEnd

		// A large receipt population can eventually leave less than one result
		// of headroom. Keep the hard bound even then.
		if sent > toolResultTranscriptBudget && generationStart < len(candidates) {
			candidate := candidates[generationStart]
			turn := &turns[candidate.index]
			sent -= len(turn.Content)
			turn.Content = candidate.receipt
			turn.Elided = max(int64(0), turn.OriginalBytes-int64(len(candidate.receipt)))
			sent += len(candidate.receipt)
			generationStart++
		}
	}
}

func toolResultBudgetReceipt(turn Turn) []byte {
	probe := formatToolResultBudgetReceipt(turn, 0)
	omitted := max(int64(0), turn.OriginalBytes-int64(len(probe)))
	return formatToolResultBudgetReceipt(turn, omitted)
}

func formatToolResultBudgetReceipt(turn Turn, omitted int64) []byte {
	return []byte(fmt.Sprintf(
		"FuseOne truncated this earlier %s result to a receipt because it did not fit the transcript result budget.\n"+
			"Original result: %d bytes, digest %s.\n"+
			"Omitted result bytes: %*d. The stored copy remains in the content store under the installation content limit. "+
			"Do not treat omitted content as absent; make another call only with a materially narrower query.",
		turn.Tool, turn.OriginalBytes, turn.ContentDigest, receiptOmittedWidth, omitted,
	))
}

func compactableLargeToolResult(tool domain.ToolID) bool {
	return compactableObservabilityTool(tool) || compactableGitHubReviewTool(tool) || compactableOutlineTool(tool)
}

func compactableObservabilityTool(tool domain.ToolID) bool {
	name := string(tool)
	if !strings.HasPrefix(name, "grafana.") {
		return false
	}
	return strings.HasPrefix(name, "grafana.query_loki") ||
		strings.HasPrefix(name, "grafana.query_prometheus")
}

func compactableGitHubReviewTool(tool domain.ToolID) bool {
	name := string(tool)
	if !strings.HasPrefix(name, "github.") {
		return false
	}
	remote := strings.TrimPrefix(name, "github.")
	return strings.HasPrefix(remote, "get_pull_request") ||
		strings.HasPrefix(remote, "list_pull_request") ||
		remote == "search_pull_requests" || remote == "get_file_contents" ||
		remote == "search_code" || remote == "get_commit" ||
		remote == "list_commits" || strings.HasSuffix(remote, "_logs")
}

func compactableOutlineTool(tool domain.ToolID) bool {
	name := strings.TrimPrefix(string(tool), "outline.")
	return name != string(tool) && (name == "fetch" || strings.HasPrefix(name, "list_") || strings.HasPrefix(name, "search"))
}

func utf8Prefix(content []byte, limit int) string {
	if len(content) <= limit {
		return string(content)
	}
	part := content[:limit]
	for len(part) > 0 && !utf8.Valid(part) {
		part = part[:len(part)-1]
	}
	return string(part)
}

func utf8Suffix(content []byte, limit int) string {
	if len(content) <= limit {
		return string(content)
	}
	part := content[len(content)-limit:]
	for len(part) > 0 && !utf8.Valid(part) {
		part = part[1:]
	}
	return string(part)
}
