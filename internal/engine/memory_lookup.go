package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
)

const (
	maxInitialMemorySearchTerms = 6
	initialMemoryFindLimit      = 10
	initialMemoryTermBytes      = 64
)

func (r *Runner) initialMemoryLookup(
	ctx context.Context, state State, start Start,
) (State, Status, bool, error) {
	if !shouldRunInitialMemoryLookup(state, start) {
		return state, Status{}, false, nil
	}
	search, err := r.initialMemorySearch(ctx, start)
	if err != nil {
		return State{}, Status{}, false, err
	}
	if search == "" {
		return state, Status{}, false, nil
	}
	args, err := json.Marshal(struct {
		Search string `json:"search"`
		Limit  int    `json:"limit"`
	}{Search: search, Limit: initialMemoryFindLimit})
	if err != nil {
		return State{}, Status{}, false, err
	}
	p := Proposal{Tool: domain.ToolMemoryFind, Args: args}
	effect, _ := r.deps.Catalog.Effect(p.Tool)
	idemKey := idempotencyKey(start.RunID, p.Tool, p.Args)
	semantic, err := r.semanticDedupe(ctx, start, p)
	if err != nil {
		return State{}, Status{}, false, err
	}
	decision, err := r.decide(ctx, state, start, p, effect, idemKey,
		state.AlreadyExecuted(idemKey) || semantic.already)
	if err != nil {
		return State{}, Status{}, false, fmt.Errorf("engine: gate: %w", err)
	}
	if !decision.Verdict.Executable() || decision.Verdict == domain.VerdictRequireApproval {
		return state, Status{}, false, nil
	}
	st, err := r.afterExecutableGate(ctx, state, start, p, effect, idemKey, semantic, decision)
	return state, st, true, err
}

func shouldRunInitialMemoryLookup(state State, start Start) bool {
	return start.MemoryLearning.Enabled() &&
		!state.Planned &&
		!slices.Contains(state.Called, domain.ToolMemoryFind) &&
		envelopeForState(start, state).Allows(domain.ToolMemoryFind)
}

func (r *Runner) initialMemorySearch(ctx context.Context, start Start) (string, error) {
	steps, err := r.deps.Ledger.Read(ctx, start.RunID, domain.FirstSeq)
	if err != nil {
		return "", fmt.Errorf("engine: read for initial memory lookup: %w", err)
	}
	turns, err := BuildTranscript(ctx, r.deps.Content, steps)
	if err != nil {
		return "", err
	}
	for _, turn := range turns {
		if turn.Kind == TurnInput {
			return initialMemorySearch(turn.Text), nil
		}
	}
	return "", nil
}

func initialMemorySearch(text string) string {
	strong, primary, secondary := initialMemoryTerms(text)
	terms := make([]string, 0, maxInitialMemorySearchTerms)
	addInitialMemoryTerms := func(values []string) {
		for _, term := range values {
			if len(terms) >= maxInitialMemorySearchTerms {
				return
			}
			if slices.Contains(terms, term) {
				continue
			}
			terms = append(terms, term)
		}
	}
	addInitialMemoryTerms(strong)
	addInitialMemoryTerms(primary)
	addInitialMemoryTerms(secondary)
	return strings.Join(terms, " ")
}

func initialMemoryTerms(text string) (strong, primary, secondary []string) {
	section := memoryLookupSectionOther
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if next, rest, ok := memoryLookupBracketSection(line); ok {
			section = next
			line = rest
		}
		if section == memoryLookupSectionSkip || strings.TrimSpace(line) == "" {
			continue
		}
		var ordinary []string
		strong, ordinary = appendInitialMemoryTokens(strong, nil, line)
		switch section {
		case memoryLookupSectionPrimary:
			primary = append(primary, ordinary...)
		default:
			secondary = append(secondary, ordinary...)
		}
	}
	return compactInitialMemoryTerms(strong), compactInitialMemoryTerms(primary),
		compactInitialMemoryTerms(secondary)
}

type memoryLookupSection uint8

const (
	memoryLookupSectionOther memoryLookupSection = iota
	memoryLookupSectionPrimary
	memoryLookupSectionSkip
)

func memoryLookupBracketSection(line string) (memoryLookupSection, string, bool) {
	if !strings.HasPrefix(line, "[") {
		return memoryLookupSectionOther, line, false
	}
	end := strings.Index(line, "]")
	if end < 0 {
		return memoryLookupSectionOther, line, false
	}
	label := strings.ToLower(line[1:end])
	label = strings.NewReplacer("(a)", "", "/", " ").Replace(label)
	label = strings.Join(strings.Fields(label), " ")
	rest := strings.TrimSpace(line[end+1:])
	switch {
	case strings.Contains(label, "autor"), strings.Contains(label, "ambiente"),
		strings.Contains(label, "observa"):
		return memoryLookupSectionSkip, rest, true
	case strings.Contains(label, "app"), strings.Contains(label, "servico"),
		strings.Contains(label, "serviço"), strings.Contains(label, "descri"):
		return memoryLookupSectionPrimary, rest, true
	default:
		return memoryLookupSectionOther, rest, true
	}
}

func appendInitialMemoryTokens(strong, ordinary []string, text string) ([]string, []string) {
	for _, raw := range strings.FieldsFunc(text, memoryLookupTokenBoundary) {
		term := normalizeInitialMemoryTerm(raw)
		if term == "" || initialMemoryStopword(term) {
			continue
		}
		if initialMemoryStrongTerm(term) {
			strong = append(strong, term)
		} else {
			ordinary = append(ordinary, term)
		}
	}
	return strong, ordinary
}

func memoryLookupTokenBoundary(r rune) bool {
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.')
}

func normalizeInitialMemoryTerm(raw string) string {
	term := strings.ToLower(strings.TrimFunc(raw, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
	}))
	if term == "" || strings.HasPrefix(term, "@") || utf8.RuneCountInString(term) < 3 {
		return ""
	}
	term = truncateInitialMemoryTerm(term)
	return term
}

func initialMemoryStrongTerm(term string) bool {
	if len(term) < 6 {
		return false
	}
	if strings.ContainsAny(term, "_.-") {
		return true
	}
	var hasLetter, hasDigit bool
	for _, r := range term {
		hasLetter = hasLetter || unicode.IsLetter(r)
		hasDigit = hasDigit || unicode.IsDigit(r)
	}
	return hasLetter && hasDigit
}

func truncateInitialMemoryTerm(term string) string {
	if len(term) <= initialMemoryTermBytes {
		return term
	}
	end := 0
	for i, r := range term {
		next := i + utf8.RuneLen(r)
		if next > initialMemoryTermBytes {
			break
		}
		end = next
	}
	return term[:end]
}

func compactInitialMemoryTerms(in []string) []string {
	out := make([]string, 0, min(len(in), maxInitialMemorySearchTerms))
	for _, term := range in {
		if slices.Contains(out, term) {
			continue
		}
		out = append(out, term)
		if len(out) == maxInitialMemorySearchTerms {
			break
		}
	}
	return out
}

func initialMemoryStopword(term string) bool {
	switch term {
	case "autor", "autora", "ambiente", "production", "prod", "servico", "serviço",
		"descricao", "descrição", "fluxo", "esperado", "observacoes", "observações",
		"adicionais", "queria", "entender", "porque", "porquê", "para", "pelo",
		"pela", "pelos", "pelas", "com", "uma", "umas", "uns", "the", "request",
		"failed", "server", "responded", "false", "error", "erro", "url", "http",
		"https", "api", "canal", "correspondente", "esta", "está", "dando":
		return true
	default:
		return false
	}
}
