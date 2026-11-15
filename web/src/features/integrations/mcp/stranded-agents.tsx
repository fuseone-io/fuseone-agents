import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import type { StrandedAgent } from "@/features/integrations/mcp/move-refusal";

/** The published agents a move would strand, by id and where they are. */
export function StrandedAgents({ agents }: { agents: StrandedAgent[] }) {
  const { t } = useTranslation();
  return (
    <div role="alert" className="space-y-2 rounded-lg border p-3 text-sm">
      <p>{t("mcp.move.strandedHeading", { count: agents.length })}</p>
      <ul className="space-y-1">
        {agents.map((agent) => (
          <li key={agent.agentId} className="flex justify-between gap-2">
            <Mono className="text-xs">{agent.agentId}</Mono>
            <Mono className="text-xs text-muted-foreground">
              {agent.area ? `${agent.company}/${agent.area}` : agent.company}
            </Mono>
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">{t("mcp.move.strandedHint")}</p>
    </div>
  );
}
