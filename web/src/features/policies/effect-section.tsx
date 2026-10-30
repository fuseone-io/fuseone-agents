import { useTranslation } from "react-i18next";
import { RadioGroup } from "@/components/ui/radio-group";
import { ChoiceCard } from "@/features/policies/choice-card";
import { Section } from "@/features/policies/section";
import type { PolicyInput } from "@/lib/api/client";

const EFFECT_CHOICES = [
  {
    value: "allow",
    label: "policies.allow",
    note: "policies.recordsAndContinues",
    className:
      "has-data-[state=checked]:border-success has-data-[state=checked]:bg-success-surface",
  },
  {
    value: "escalate",
    label: "policies.escalate",
    note: "policies.stopsForAPerson",
    className:
      "has-data-[state=checked]:border-warning has-data-[state=checked]:bg-warning-surface",
  },
  {
    value: "deny",
    label: "policies.deny",
    note: "policies.refusesAndRecords",
    className:
      "has-data-[state=checked]:border-danger has-data-[state=checked]:bg-danger-surface",
  },
] as const;

const MODES = [
  { value: "monitor", label: "policies.monitor" },
  { value: "enforce", label: "policies.enforce" },
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
      <RadioGroup
        aria-label={t("policies.effectWhenMatched")}
        value={draft.effect}
        onValueChange={(effect) =>
          patch({ effect: effect as PolicyInput["effect"] })
        }
        className="grid min-w-0 gap-2 sm:grid-cols-[repeat(3,minmax(0,1fr))]"
      >
        {EFFECT_CHOICES.map((choice) => (
          <ChoiceCard
            key={choice.value}
            value={choice.value}
            label={t(choice.label)}
            note={t(choice.note)}
            className={`p-3 ${choice.className}`}
          />
        ))}
      </RadioGroup>

      <div>
        <RadioGroup
          aria-label={t("policies.enforcement")}
          value={draft.mode}
          onValueChange={(mode) => patch({ mode: mode as PolicyInput["mode"] })}
          className="flex min-w-0 flex-wrap gap-2"
        >
          {MODES.map((mode) => (
            <ChoiceCard
              key={mode.value}
              value={mode.value}
              label={t(mode.label)}
              className="min-w-28 flex-1 rounded-md px-2 py-1.5 has-data-[state=checked]:border-primary has-data-[state=checked]:bg-surface-accent has-data-[state=checked]:text-text-accent"
            />
          ))}
        </RadioGroup>
        <p className="mt-1.5 text-xs text-muted-foreground">
          {t("policies.monitoring")}
        </p>
      </div>
    </Section>
  );
}
