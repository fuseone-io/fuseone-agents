import { CircleDollarSign } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useRegressions } from "@/features/agents/regressions-api";
import { formatMicros, formatTokens } from "@/lib/format";
import type { Agent } from "@/lib/api/client";

type Source = "corpus" | "write" | "json";

export function SimulationCostPreview({
  agentId,
  agent,
  source,
  chosenCount,
}: {
  agentId: string;
  agent?: Agent;
  source: Source;
  chosenCount: number;
}) {
  const { t } = useTranslation();
  const corpus = useRegressions(agentId, source === "corpus");
  const caseCount =
    source === "corpus" ? corpus.data?.items.length : chosenCount;
  const perRunMicros = agent?.budget.micros ?? 0;
  const totalMicros =
    perRunMicros > 0 && caseCount !== undefined
      ? perRunMicros * caseCount
      : undefined;
  const averageMicros =
    agent?.activity?.runs && agent.activity.costMicros > 0
      ? Math.round(agent.activity.costMicros / agent.activity.runs)
      : undefined;
  const expectedMicros =
    averageMicros !== undefined && caseCount !== undefined && caseCount > 0
      ? averageMicros * caseCount
      : undefined;

  return (
    <div className="flex min-w-0 items-start gap-3 rounded-lg border bg-muted p-4">
      <CircleDollarSign
        className="mt-0.5 size-4 shrink-0 text-text-accent"
        aria-hidden
      />
      <div className="min-w-0 space-y-1">
        <p className="text-sm font-medium">{t("simulation.costPreviewTitle")}</p>
        <p className="text-sm text-muted-foreground">
          {costPreviewText({
            t,
            source,
            caseCount,
            perRunMicros,
            totalMicros,
            averageMicros,
            expectedMicros,
            loading: source === "corpus" && corpus.isLoading,
            error: source === "corpus" && corpus.isError,
          })}
        </p>
        <p className="text-xs text-muted-foreground">
          {budgetShape(agent, t)}
        </p>
      </div>
    </div>
  );
}

function costPreviewText({
  t,
  source,
  caseCount,
  perRunMicros,
  totalMicros,
  averageMicros,
  expectedMicros,
  loading,
  error,
}: {
  t: ReturnType<typeof useTranslation>["t"];
  source: Source;
  caseCount?: number;
  perRunMicros: number;
  totalMicros?: number;
  averageMicros?: number;
  expectedMicros?: number;
  loading: boolean;
  error: boolean;
}) {
  if (source === "corpus" && loading) {
    if (perRunMicros <= 0) return t("simulation.costPreviewCountingUnbounded");
    return t("simulation.costPreviewCounting", {
      perRun: formatMicros(perRunMicros),
    });
  }
  if (source === "corpus" && error) {
    if (perRunMicros <= 0) return t("simulation.costPreviewCorpusUnknownUnbounded");
    return t("simulation.costPreviewCorpusUnknown", {
      perRun: formatMicros(perRunMicros),
    });
  }
  if (caseCount === undefined || caseCount <= 0) {
    if (perRunMicros <= 0) return t("simulation.costPreviewUnbounded");
    return t("simulation.costPreviewNoCases", {
      perRun: formatMicros(perRunMicros),
    });
  }
  if (expectedMicros !== undefined && averageMicros !== undefined) {
    if (totalMicros !== undefined) {
      if (averageMicros > perRunMicros) {
        return t("simulation.costPreviewAverageAboveCeiling", {
          count: caseCount,
          average: formatMicros(averageMicros),
          perRun: formatMicros(perRunMicros),
          total: formatMicros(totalMicros),
        });
      }
      return t("simulation.costPreviewExpectedBounded", {
        count: caseCount,
        average: formatMicros(averageMicros),
        expected: formatMicros(expectedMicros),
        perRun: formatMicros(perRunMicros),
        total: formatMicros(totalMicros),
      });
    }
    return t("simulation.costPreviewExpectedUnbounded", {
      count: caseCount,
      average: formatMicros(averageMicros),
      expected: formatMicros(expectedMicros),
    });
  }
  if (perRunMicros <= 0) return t("simulation.costPreviewUnbounded");
  return t("simulation.costPreviewBounded", {
    count: caseCount,
    perRun: formatMicros(perRunMicros),
    total: formatMicros(totalMicros ?? 0),
  });
}

function budgetShape(agent: Agent | undefined, t: ReturnType<typeof useTranslation>["t"]) {
  const budget = agent?.budget;
  if (!budget) return t("simulation.costPreviewBudgetUnknown");
  const parts = [
    budget.steps ? t("simulation.costPreviewSteps", { n: formatTokens(budget.steps) }) : "",
    budget.tokens ? t("simulation.costPreviewTokens", { n: formatTokens(budget.tokens) }) : "",
    budget.toolCalls
      ? t("simulation.costPreviewToolCalls", { n: formatTokens(budget.toolCalls) })
      : "",
  ].filter(Boolean);
  if (budget.micros) {
    parts.unshift(t("simulation.costPreviewMoney", { amount: formatMicros(budget.micros) }));
  }
  if (parts.length === 0) return t("simulation.costPreviewNoRunCeiling");
  return t("simulation.costPreviewBudget", { budget: parts.join(" · ") });
}
