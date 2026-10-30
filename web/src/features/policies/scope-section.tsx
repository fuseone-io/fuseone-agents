import { useTranslation } from "react-i18next";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Section } from "@/features/policies/section";
import { ToolField } from "@/features/policies/tool-field";
import type { PolicyInput } from "@/lib/api/client";

const COVERED = ["read", "write", "destructive", "financial"] as const;

/** Scope: what the rule covers before any condition is read. */
export function ScopeSection({
  draft,
  patch,
}: {
  draft: PolicyInput;
  patch: (over: Partial<PolicyInput>) => void;
}) {
  const { t } = useTranslation();
  const covered = draft.effects ?? [];

  return (
    <Section title={t("policies.scope")} hint={t("policies.scopeReach")}>
      <ToolField
        value={draft.resource ?? ""}
        onChange={(resource) => patch({ resource })}
      />

      <fieldset className="flex flex-col gap-1.5">
        <legend className="text-2xs uppercase tracking-label text-muted-foreground">
          {t("policies.effectsCovered")}
        </legend>
        {/* Many-of-many, so a toggle group rather than radios: each effect is
            pressed on its own, and the order they were pressed in is kept. */}
        <ToggleGroup
          type="multiple"
          variant="outline"
          spacing={1.5}
          value={covered}
          onValueChange={(effects) =>
            patch({ effects: effects as PolicyInput["effects"] })
          }
          className="flex-wrap"
        >
          {COVERED.map((effect) => (
            <ToggleGroupItem
              key={effect}
              value={effect}
              className="h-7 rounded-pill px-2.5 font-mono text-2xs data-[state=on]:border-primary data-[state=on]:bg-surface-accent data-[state=on]:text-text-accent"
            >
              {effect}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <p className="text-xs text-muted-foreground">
          {t("policies.noneSelectedCoversAll")}
        </p>
      </fieldset>
    </Section>
  );
}
