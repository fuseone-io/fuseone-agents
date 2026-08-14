import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useSetAgentPaused } from "@/features/agents/agent-editor-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Whether the agent is running, and the one control that changes it.
 *
 * An agent is published stopped and starts only because somebody decided it
 * should, so this belongs next to the version that was just published — a
 * screen that can publish but not start leaves the author looking for a
 * button that is not there.
 *
 * Starting is refused while the corrections this version was last simulated
 * against no longer hold, and the server's sentence names them. Stopping is
 * never refused.
 */
export function PauseControl({
  agentId,
  paused,
}: {
  agentId: string;
  paused: boolean | undefined;
}) {
  const { t } = useTranslation();
  const set = useSetAgentPaused(agentId);
  const running = paused === false;

  const change = (next: boolean) =>
    set.mutate(!next, {
      onSuccess: () => toast.success(t(next ? "agents.started" : "agents.stopped")),
      onError: (error) =>
        toast.error(t(next ? "agents.startFailed" : "agents.stopFailed"), {
          description: problemMessage(error, t),
        }),
    });

  return (
    <div className="flex items-center gap-2">
      <Switch
        id={`running-${agentId}`}
        checked={running}
        onCheckedChange={change}
        disabled={set.isPending}
      />
      {/* The word beside the switch, never the switch alone: an agent that is
          stopped and an agent that is running look the same at a glance. */}
      <Label htmlFor={`running-${agentId}`} className="text-sm font-normal">
        {t(running ? "agents.running" : "agents.stopped")}
      </Label>
    </div>
  );
}
