import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { SummaryFact, SummarySection } from "@/features/workbench/summary-section";
import { SummaryFindings } from "@/features/workbench/summary-findings";
import { SummaryInstructions } from "@/features/workbench/summary-instructions";
import { SummaryTokens } from "@/features/workbench/summary-tokens";
import { SummaryTools } from "@/features/workbench/summary-tools";
import { WorkbenchMissing } from "@/features/workbench/workbench-missing";
import type { FieldPart } from "@/features/workbench/workbench-field-part";
import type { AgentDefinition, AgentDraft } from "@/lib/api/client";
import { formatMicros } from "@/lib/format";

/**
 * The draft as NT-018's mockup shows it: compact and read-only, what the
 * person changed and has not saved included. Each part opens its manual
 * controls; the instructions have none, since they change only through the
 * conversation.
 */
export function WorkbenchDraftSummary({
  draft,
  local,
  published,
  onEdit,
  patch,
}: {
  draft: AgentDraft;
  local: AgentDefinition;
  /** The published instructions, when there is a published version. */
  published?: string;
  onEdit: (part: FieldPart) => void;
  /** For the one field a finding fixes in place: enabling a tool it names. */
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const none = t("workbench.summary.none");
  const model = [local.provider, local.model, local.effort].filter(Boolean).join(" · ");
  const budget = local.budget;

  return (
    <div className="flex flex-col gap-6 p-4">
      <p className="text-xs text-muted-foreground">
        {t(draft.baseVersionId ? "workbench.summary.againstPublished" : "workbench.summary.neverPublished")}
      </p>
      <WorkbenchMissing draft={draft} />
      <SummarySection title={t("workbench.parts.identity")} onEdit={() => onEdit("identity")}>
        <dl className="flex flex-col gap-1.5">
          <SummaryFact label={t("workbench.summary.name")}>{local.name || none}</SummaryFact>
          <SummaryFact label={t("workbench.summary.scope")}>
            <Mono>{[local.company, local.area].filter(Boolean).join(" · ")}</Mono>
          </SummaryFact>
          <SummaryFact label={t("workbench.summary.model")}>{model ? <Mono>{model}</Mono> : none}</SummaryFact>
        </dl>
      </SummarySection>
      <SummarySection title={t("workbench.parts.tools")} onEdit={() => onEdit("tools")}>
        <SummaryTools local={local} />
      </SummarySection>
      <SummarySection title={t("workbench.parts.governance")} onEdit={() => onEdit("governance")}>
        <dl className="flex flex-col gap-1.5">
          <SummaryFact label={t("workbench.summary.budget")}>
            {budget?.micros ? <Mono>{formatMicros(budget.micros)}</Mono> : none}
            {budget?.steps ? <Mono dim> · {t("workbench.summary.steps", { count: budget.steps })}</Mono> : null}
          </SummaryFact>
          <SummaryFact label={t("workbench.summary.triggers")}>
            {t("workbench.summary.triggerCount", { count: local.triggers?.length ?? 0 })}
          </SummaryFact>
        </dl>
      </SummarySection>
      <SummarySection title={t("workbench.parts.steps")} onEdit={() => onEdit("steps")}>
        {(local.steps ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("workbench.summary.noSteps")}</p>
        ) : (
          <ol className="flex flex-col gap-1 text-sm">
            {(local.steps ?? []).map((step, at) => (
              <li key={`${at}:${step.name}`}>{step.name}</li>
            ))}
          </ol>
        )}
      </SummarySection>
      <SummarySection title={t("workbench.instructionsTitle")}>
        <p className="text-xs text-muted-foreground">{t("workbench.instructionsNote")}</p>
        <SummaryTokens local={local} />
        <SummaryFindings local={local} patch={patch} />
        <SummaryInstructions now={draft.definition.instructions} published={published} />
      </SummarySection>
    </div>
  );
}
