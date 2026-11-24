import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { EffectBadge } from "@/features/admin/effect-badge";
import { Badge } from "@/components/ui/badge";
import { useTools } from "@/features/admin/api";
import { unplacedTools } from "@/features/agents/steps-unplaced";
import { agentTarget } from "@/features/agents/agent-target";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * The tools the draft grants, each with the effect its scope's catalogue rules
 * for it, and marked when no step reaches it: a run never receives that tool.
 */
export function SummaryTools({ local }: { local: AgentDefinition }) {
  const { t } = useTranslation();
  const catalogue = useTools(local.company !== "", agentTarget(local));
  const granted = local.tools ?? [];
  if (granted.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("workbench.summary.noTools")}</p>;
  }
  const unplaced = new Set(unplacedTools(granted, local.steps ?? []));
  const effects = new Map((catalogue.data?.items ?? []).map((tool) => [tool.toolId, tool.effect] as const));
  return (
    <ul className="flex flex-col gap-1.5">
      {granted.map((tool) => {
        const effect = effects.get(tool);
        return (
          <li key={tool} className="flex items-center gap-2">
            <Mono className="text-sm">{tool}</Mono>
            {effect && <EffectBadge effect={effect} />}
            {unplaced.has(tool) && (
              <Badge variant="outline" className="border-warning text-warning">
                {t("workbench.summary.unplaced")}
              </Badge>
            )}
          </li>
        );
      })}
    </ul>
  );
}
