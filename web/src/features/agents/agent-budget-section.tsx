import { ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Section, Labelled } from "@/features/policies/section";
import { agentRequirementMarked } from "@/features/agents/agent-required";
import { formatMicros } from "@/lib/format";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * What one run may spend before it is stopped.
 *
 * Per run, not per day: a ceiling across a day is a budget, and budgets are set
 * per scope in Administração. This is what stops one run looping.
 */
export function AgentBudgetSection({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const budget = draft.budget ?? {};
  const set = (over: Partial<typeof budget>) =>
    patch({ budget: { ...budget, ...over } });

  return (
    <Section
      icon={ShieldCheck}
      title={t("agents.perRunCeiling")}
      hint={t("agents.zeroIsNoCeiling")}
      required={agentRequirementMarked("budget")}
    >
      <div className="grid gap-3 sm:grid-cols-4">
        <Labelled label={t("agents.costMicros")} htmlFor="budget-micros">
          <Input
            id="budget-micros"
            type="number"
            value={budget.micros ?? 0}
            onChange={(e) => set({ micros: Number(e.target.value) })}
            className="font-mono"
          />
          <span className="text-2xs text-muted-foreground">
            {formatMicros(budget.micros ?? 0)}
          </span>
        </Labelled>
        <Labelled label={t("runs.columnSteps")} htmlFor="budget-steps">
          <Input
            id="budget-steps"
            type="number"
            value={budget.steps ?? 0}
            onChange={(e) => set({ steps: Number(e.target.value) })}
            className="font-mono"
          />
        </Labelled>
        <Labelled label={t("agents.calls")} htmlFor="budget-calls">
          <Input
            id="budget-calls"
            type="number"
            value={budget.toolCalls ?? 0}
            onChange={(e) => set({ toolCalls: Number(e.target.value) })}
            className="font-mono"
          />
        </Labelled>
        <Labelled label={t("runs.kpiTokens")} htmlFor="budget-tokens">
          <Input
            id="budget-tokens"
            type="number"
            value={budget.tokens ?? 0}
            onChange={(e) => set({ tokens: Number(e.target.value) })}
            className="font-mono"
          />
        </Labelled>
      </div>

      <p className="text-xs text-muted-foreground">
        {t("agents.budgetNeeded")}
      </p>
    </Section>
  );
}
