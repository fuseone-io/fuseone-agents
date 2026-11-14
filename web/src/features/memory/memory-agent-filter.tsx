import { useTranslation } from "react-i18next";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAgents } from "@/features/agents/api";

/** Radix reserves the empty value; this stands for "no filter". */
const ALL = "__all__";

/**
 * Which agent's memory to show, chosen from the agents rather than typed.
 *
 * A free id with a case typo silently showed nothing. "All agents" sends no
 * filter at all. A filter already set to an id the listing does not hold is
 * kept as its own item, so the control never shows a value it is not sending.
 */
export function MemoryAgentFilter({
  agentId,
  onChange,
}: {
  agentId: string;
  onChange: (agentId: string) => void;
}) {
  const { t } = useTranslation();
  const agents = useAgents().data?.items ?? [];
  const listed = agents.some((agent) => agent.agentId === agentId);
  return (
    <Select
      value={agentId || ALL}
      onValueChange={(next) => onChange(next === ALL ? "" : next)}
    >
      <SelectTrigger
        aria-label={t("memory.agentFilter")}
        className="!h-8 min-w-44 flex-1 sm:max-w-56"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>{t("memory.allAgents")}</SelectItem>
        {agentId !== "" && !listed && (
          <SelectItem value={agentId} className="font-mono">
            {agentId}
          </SelectItem>
        )}
        {agents.map((agent) => (
          <SelectItem key={agent.agentId} value={agent.agentId}>
            {agent.name || agent.agentId}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
