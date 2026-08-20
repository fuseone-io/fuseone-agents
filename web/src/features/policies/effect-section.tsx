import { useTranslation } from "react-i18next";
import { Section } from "@/features/policies/section";
import { cn } from "@/lib/utils";
import type { PolicyInput } from "@/lib/api/client";

const EFFECT_CHOICES = [
  {
    value: "allow",
    label: "policies.allow",
    note: "policies.recordsAndContinues",
    className: "border-success bg-success-surface",
  },
  {
    value: "escalate",
    label: "policies.escalate",
    note: "policies.stopsForAPerson",
    className: "border-warning bg-warning-surface",
  },
  {
    value: "deny",
    label: "policies.deny",
    note: "policies.refusesAndRecords",
    className: "border-danger bg-danger-surface",
  },
] as const;

/** What happens on a match, and whether anybody obeys it. */
export function EffectSection({
  draft,
  patch,
}: {
  draft: PolicyInput;
  patch: (over: Partial<PolicyInput>) => void;
}) {
  const { t } = useTranslation();
  return (
    <Section title={t("policies.effectAndEnforcement")}>
      <fieldset>
        <legend className="sr-only">{t("policies.effectWhenMatched")}</legend>
        <div className="grid gap-2 sm:grid-cols-3">
          {EFFECT_CHOICES.map((choice) => (
            <button
              key={choice.value}
              type="button"
              role="radio"
              aria-checked={draft.effect === choice.value}
              onClick={() => patch({ effect: choice.value })}
              className={cn(
                "flex flex-col gap-0.5 rounded-lg border p-3 text-left",
                draft.effect === choice.value
                  ? choice.className
                  : "border-border",
              )}
            >
              <span className="text-sm font-medium">{t(choice.label)}</span>
              <span className="text-xs text-muted-foreground">
                {t(choice.note)}
              </span>
            </button>
          ))}
        </div>
      </fieldset>

      <fieldset>
        <legend className="sr-only">{t("policies.enforcement")}</legend>
        <div className="flex gap-2">
          {(["monitor", "enforce"] as const).map((mode) => (
            <button
              key={mode}
              type="button"
              role="radio"
              aria-checked={draft.mode === mode}
              onClick={() => patch({ mode })}
              className={cn(
                "h-8 flex-1 rounded-md border text-xs",
                draft.mode === mode
                  ? "border-primary bg-surface-accent text-text-accent"
                  : "border-border",
              )}
            >
              {t(mode === "monitor" ? "policies.monitor" : "policies.enforce")}
            </button>
          ))}
        </div>
        <p className="mt-1.5 text-xs text-muted-foreground">
          {t("policies.monitoring")}
        </p>
      </fieldset>
    </Section>
  );
}
