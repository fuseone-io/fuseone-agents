import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { ConditionBuilder } from "@/features/policies/condition-builder";
import { draftSentence } from "@/features/policies/policy-sentence";
import { Section } from "@/features/policies/section";
import type { PolicyInput } from "@/lib/api/client";

/** The rule itself, and the line it compiles to. */
export function ConditionSection({
  draft,
  patch,
}: {
  draft: PolicyInput;
  patch: (over: Partial<PolicyInput>) => void;
}) {
  const { t } = useTranslation();
  return (
    <Section
      title={t("policies.condition")}
      hint="Todas precisam ser verdadeiras."
    >
      <ConditionBuilder
        conditions={draft.conditions ?? []}
        onChange={(conditions) => patch({ conditions })}
      />

      {/* Never the only representation: the author reads the sentence the
          engine will evaluate, not just the rows they clicked together. */}
      <div className="rounded-lg border border-border bg-muted p-3">
        <span className="text-2xs uppercase tracking-label text-muted-foreground">
          {t("policies.ruleEvaluated")}
        </span>
        <Mono className="mt-1 block break-words text-xs">
          {draftSentence(draft)}
        </Mono>
      </div>
    </Section>
  );
}
