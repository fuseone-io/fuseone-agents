package slack

import (
	"strings"
	"testing"
)

/*
The partition invariant is what this feature is.

The formatted render contains exactly the same translated characters as the
single-block render, only partitioned at blank lines: rejoining the sections
with the separators they were split on reconstitutes outcome(text), modulo the
collapse of consecutive blank lines (two paragraphs separated by three blank
lines rejoin with one separator — the characters removed are only blank
separators, never content).

Any "improvement" — a bolded headline, a demoted parenthesis, a dropped line —
breaks this test, which is what keeps typography from quietly becoming
interpretation. The doctrine is rendered structure, never guessed meaning.
*/
func TestAnswerSections_isAPartitionOfTheSingleBlockRender(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		sweepReport,
		runbookDiagnosis,
		"one paragraph only, no breaks",
		"para one\n\npara two\n\n\n\npara three after many blanks",
		"before\n\n```\ncode with\n\nblank inside\n```\n\nafter",
		"• a\n• b\n• c\n\ntail",
	} {
		sections := answerSections(text)
		rejoined := strings.Join(sections, "\n\n")
		want := collapseBlankRuns(outcome(text))
		if rejoined != want {
			t.Errorf("partition changed the text.\n got: %q\nwant: %q", rejoined, want)
		}
	}
}

// A blank line inside a code fence is content, not a paragraph break: a fence
// split in half renders as two broken fences, which rewrites what the agent
// wrote.
func TestAnswerSections_aFenceWithABlankLineInside_staysOneSection(t *testing.T) {
	t.Parallel()
	sections := answerSections("before\n\n```\ntop\n\nbottom\n```\n\nafter")
	if len(sections) != 3 {
		t.Fatalf("sections = %d (%q), want before/fence/after", len(sections), sections)
	}
	if !strings.Contains(sections[1], "top") || !strings.Contains(sections[1], "bottom") {
		t.Errorf("the fence was split: %q", sections[1])
	}
}

// A run of bullet lines is one list, not one block per bullet.
func TestAnswerSections_aBulletRun_staysTogether(t *testing.T) {
	t.Parallel()
	sections := answerSections("head\n\n• one\n• two\n• three\n\ntail")
	if len(sections) != 3 {
		t.Fatalf("sections = %d (%q)", len(sections), sections)
	}
	if strings.Count(sections[1], "•") != 3 {
		t.Errorf("the list was split: %q", sections[1])
	}
}

// More paragraphs than Slack can hold as blocks are merged into the last
// section, never dropped: the vendor's ceiling bounds the shape, not the
// content.
func TestAnswerSections_pastTheBlockCeiling_theTailIsMergedNotDropped(t *testing.T) {
	t.Parallel()
	parts := make([]string, 0, 700)
	for range 700 {
		parts = append(parts, "x")
	}
	sections := answerSections(strings.Join(parts, "\n\n"))
	if len(sections) > maxAnswerSections {
		t.Fatalf("sections = %d, ceiling is %d", len(sections), maxAnswerSections)
	}
	total := 0
	for _, s := range sections {
		total += strings.Count(s, "x")
	}
	if total != 700 {
		t.Errorf("content was dropped: %d of 700 survived", total)
	}
}

// collapseBlankRuns is the rejoin's counterpart: splitting on blank runs and
// joining with one separator is lossless except for the width of the runs.
func collapseBlankRuns(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.Trim(s, "\n")
}

const sweepReport = `✅ Varredura 21:01–22:01 (BRT): tudo ok, sem achados.

Detalhe operacional (V1–V11):
• V1: zero LOGIN_ERROR na janela.
• V2: 1 CLIENT_LOGIN_ERROR de client int- (realm integration, fora do escopo).
• V3: password grant só em application-access (5.963) no realm cora.

Nenhuma ação tomada (somente leitura); nenhum IP atingiu critério de bloqueio.`

const runbookDiagnosis = `Runbook consultado: alertGatewayRTMInterfaceErrors.

Diagnóstico: alerta crítico indica anomalia na taxa de erros 5xx do job
nginx-third-part para o vhost cora.prod.jdpi.pstijd.

Causas prováveis (conforme runbook):
- Problema no próprio Gateway ao processar requests.
- Timeout do lado da JD.

Próximos passos recomendados:
1. Verificar se alertGatewayRTMHeapHigh também está firing.
2. Checar console do Gateway Third Part nos logs do endpoint.`
