package engine

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
)

const (
	toolResultCompactAfter = 32 << 10
	toolResultHeadBytes    = 16 << 10
	toolResultTailBytes    = 8 << 10
	// 128 KiB, sized so one disciplined checklist run — a dozen-plus small
	// results and a few compacted deep dives — reaches its final composition
	// whole. The old 64 KiB was sized when every turn repaid these bytes at
	// full price; under the prefix cache the resend costs a tenth, and a
	// report blind to results it already read buys the wrong economy (#163).
	toolResultTranscriptBudget = 128 << 10
	// receiptOmittedWidth makes every int64 rendering the same width. The
	// receipt computes omitted bytes from a zero-valued probe; without a fixed
	// width, writing the real number would change the receipt length and make
	// that count wrong.
	receiptOmittedWidth = 20
	// headroomVictimMin is the smallest saving worth demoting a result for
	// beyond the hard bound. Draining deeper at a crossing is free for the
	// prefix cache — the crossing already invalidated it — but a small result
	// is a verdict the final composition needs, so headroom is only bought
	// with bulk.
	headroomVictimMin = 4 << 10
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

/*
boundToolResultTranscript keeps result bytes bounded, evicting the largest
results first.

Age-based eviction was blind in exactly the case the budget exists for: a
checklist run returns many small verdicts early and a few large deep dives
late, and demoting the oldest ate the verdicts while the already-analysed
bulk survived — the final report then called kilobyte results unreadable
(#163). The largest result is the one whose eviction buys the most headroom
and whose contents the model has already mined; ties go to the oldest.

The crossings stay coarse for the provider's prefix cache: a crossing demotes
down to half the budget at once, so growth between crossings is free, and the
decision for every prefix of results depends only on that prefix — rebuilding
a longer transcript never revisits what an earlier build demoted.

The newest result is kept whole — it is the evidence the model just asked for
— unless it alone breaks the hard bound. The stored copy remains subject to
the installation content limit, and Elided is set from the original result so
run-spend diagnostics attribute the saving to the tool that produced it.
*/
func boundToolResultTranscript(turns []Turn) {
	type candidate struct {
		index   int
		receipt []byte
	}
	demote := func(c candidate, sent int) int {
		turn := &turns[c.index]
		sent -= len(turn.Content)
		turn.Content = c.receipt
		turn.Elided = max(int64(0), turn.OriginalBytes-int64(len(c.receipt)))
		return sent + len(c.receipt)
	}

	var live []candidate
	sent := 0
	for i := range turns {
		if turns[i].Kind != TurnToolResult || len(turns[i].Content) == 0 {
			continue
		}
		receipt := toolResultBudgetReceipt(turns[i])
		if len(receipt) >= len(turns[i].Content) {
			receipt = turns[i].Content
		}
		live = append(live, candidate{index: i, receipt: receipt})
		sent += len(turns[i].Content)
		if sent <= toolResultTranscriptBudget {
			continue
		}

		for sent > toolResultTranscriptBudget/2 {
			largest := -1
			for j := 0; j < len(live)-1; j++ {
				// Only a result whose receipt is smaller than it can buy
				// headroom; a tiny result's receipt is itself.
				if len(live[j].receipt) >= len(turns[live[j].index].Content) {
					continue
				}
				if largest == -1 ||
					len(turns[live[j].index].Content) > len(turns[live[largest].index].Content) {
					largest = j
				}
			}
			if largest == -1 {
				break
			}
			// A small result is only demoted when the hard bound demands it,
			// never for headroom: the small answers are what the final
			// composition needs, and buying future slack with them is the
			// age-based blindness again, one size class down (#163).
			saving := len(turns[live[largest].index].Content) - len(live[largest].receipt)
			if sent <= toolResultTranscriptBudget && saving < headroomVictimMin {
				break
			}
			sent = demote(live[largest], sent)
			live = append(live[:largest], live[largest+1:]...)
		}

		// One result alone can exceed the aggregate ceiling when its tool has
		// no safe per-result compactor. A receipt is the only bounded form.
		if sent > toolResultTranscriptBudget {
			newest := live[len(live)-1]
			if len(newest.receipt) < len(turns[newest.index].Content) {
				sent = demote(newest, sent)
				live = live[:len(live)-1]
			}
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
