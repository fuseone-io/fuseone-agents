import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Play, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useSetAgentPaused, useSetAgentRetired } from "@/features/agents/agent-editor-api";
import { RunNowDialog } from "@/features/agents/run-now-dialog";
import { problemMessage } from "@/lib/api/problem-message";
import type { Agent } from "@/lib/api/client";

/**
 * The one filled button, and the only one in the bar.
 *
 * Its position never moves and its verb follows the state, so somebody learns
 * one place rather than four. A stopped agent is asked to start, because
 * starting is the consequential act and the one the corpus gate stands in
 * front of; a running one is asked to run now, because with the agent live
 * that is the thing somebody came here to do.
 */
export function AgentPrimary({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const start = useSetAgentPaused(agent.agentId);
  const restore = useSetAgentRetired(agent.agentId);

  if (agent.retired) {
    return (
      <Button
        size="sm"
        className="h-9"
        disabled={restore.isPending}
        onClick={() =>
          restore.mutate(false, {
            onSuccess: () => toast.success(t("agents.restored")),
            onError: (error) => toast.error(problemMessage(error, t)),
          })
        }
      >
        <RotateCcw className="size-4" aria-hidden />
        {t("agents.restore")}
      </Button>
    );
  }

  if (agent.paused !== false) {
    return (
      <Button
        size="sm"
        className="h-9"
        disabled={start.isPending}
        onClick={() =>
          start.mutate(false, {
            onSuccess: () => toast.success(t("agents.started")),
            // The corpus gate refuses here, naming the corrections that
            // stopped holding. Its sentence is the useful one.
            onError: (error) =>
              toast.error(t("agents.startFailed"), {
                description: problemMessage(error, t),
              }),
          })
        }
      >
        <Play className="size-4" aria-hidden />
        {t("agents.start")}
      </Button>
    );
  }

  return <RunNowDialog agentId={agent.agentId} agentName={agent.name} />;
}
