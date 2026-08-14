import { useTranslation } from "react-i18next";
import { StateDot } from "@/components/shared/state-dot";
import { formatRelative } from "@/lib/format";
import type { Agent } from "@/lib/api/client";

/**
 * Whether the agent is on, read rather than operated.
 *
 * It used to be a switch sitting next to the word "Stopped", which said
 * neither thing clearly: a control that looks like state and a state that
 * looks like a control. Turning an agent on is a consequence of pressing
 * something with a verb on it, and this is the reading.
 *
 * The second line is what makes the first one worth anything. "Stopped" alone
 * leaves somebody wondering whether that is normal; "stopped, nothing since
 * 09:12" answers it.
 */
export function AgentStateBlock({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const retired = agent.retired ?? false;
  const running = !retired && agent.paused === false;

  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <StateDot state={retired ? "draft" : running ? "running" : "blocked"} />
      <div className="min-w-0">
        <p className="text-sm font-medium whitespace-nowrap">
          {retired
            ? t("agents.retiredState")
            : running
              ? t("agents.running")
              : t("agents.stopped")}
        </p>
        <p className="text-2xs whitespace-nowrap text-muted-foreground">
          {agent.activity?.lastRunAt
            ? t("agents.lastRunAt", {
                when: formatRelative(agent.activity.lastRunAt),
              })
            : t("agents.neverRan")}
        </p>
      </div>
    </div>
  );
}
