import { Plus, Zap } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Section } from "@/features/policies/section";
import { TriggerRow } from "@/features/agents/trigger-row";
import { TRIGGER_KINDS, emptyTrigger } from "@/features/agents/trigger-kinds";
import type { AgentDefinition, AgentTrigger } from "@/lib/api/client";

/**
 * What starts a run without anybody asking.
 *
 * The contract has carried triggers since the beginning and no screen offered
 * them, so every agent authored in the console could only ever be started by
 * hand — the declaration existed in the file format and nowhere a person could
 * reach it.
 *
 * Nothing here grants an agent anything. A trigger decides *when* it runs; what
 * it may do when it does is the pack and the policies, and an agent triggered
 * every minute is still refused every effect it was never allowed.
 */
export function AgentTriggersSection({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const triggers = draft.triggers ?? [];

  const replace = (at: number, over: Partial<AgentTrigger>) =>
    patch({
      triggers: triggers.map((trigger, i) =>
        i === at ? { ...trigger, ...over } : trigger,
      ),
    });

  return (
    <Section
      icon={Zap}
      title={t("agents.triggers")}
      hint={t("agents.triggersHint")}
    >
      {triggers.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("agents.manualOnly")}
        </p>
      ) : (
        <div className="flex flex-col gap-2">
          {triggers.map((trigger, at) => (
            <TriggerRow
              key={at}
              trigger={trigger}
              onChange={(over) => replace(at, over)}
              onRemove={() =>
                patch({ triggers: triggers.filter((_, i) => i !== at) })
              }
            />
          ))}
        </div>
      )}

      <div className="flex gap-2">
        {TRIGGER_KINDS.map((kind) => (
          <Button
            key={kind}
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              patch({ triggers: [...triggers, emptyTrigger(kind)] })
            }
          >
            <Plus className="size-3.5" />
            {t(`agents.trigger.${kind}`)}
          </Button>
        ))}
      </div>
    </Section>
  );
}
