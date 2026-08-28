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

// boundToolResultTranscript keeps recent evidence useful while older results
// become receipts. The stored copy remains subject to the installation content
// limit, and Elided is set from the original result so run-spend diagnostics
// attribute the saving to the tool that produced it.
func boundToolResultTranscript(turns []Turn) {
	type candidate struct {
		index   int
		receipt []byte
	}
	var candidates []candidate
	base := 0
	for i := range turns {
		if turns[i].Kind != TurnToolResult || len(turns[i].Content) == 0 {
			continue
		}
		receipt := toolResultBudgetReceipt(turns[i])
		if len(receipt) >= len(turns[i].Content) {
			receipt = turns[i].Content
		}
		candidates = append(candidates, candidate{index: i, receipt: receipt})
		base += len(receipt)
	}
	remaining := max(0, toolResultTranscriptBudget-base)
	for i := len(candidates) - 1; i >= 0; i-- {
		candidate := candidates[i]
		turn := &turns[candidate.index]
		extra := len(turn.Content) - len(candidate.receipt)
		if extra <= remaining {
			remaining -= extra
			continue
		}
		turn.Content = candidate.receipt
		turn.Elided = max(int64(0), turn.OriginalBytes-int64(len(candidate.receipt)))
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
			"Omitted result bytes: %20d. The stored copy remains in the content store under the installation content limit. "+
			"Do not treat omitted content as absent; make another call only with a materially narrower query.",
		turn.Tool, turn.OriginalBytes, turn.ContentDigest, omitted,
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
