import { useTranslation } from "react-i18next";
import { Archive, Ellipsis, Pause } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useSetAgentPaused } from "@/features/agents/agent-editor-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Everything that must not sit eight pixels from the primary action.
 *
 * Retiring an agent is the clearest case: it is the one act here somebody
 * cannot take back by pressing the same button again, and a row that put it
 * next to Run would eventually have somebody press the wrong one. Stopping
 * lives here for a milder version of the same reason — it is a decision about
 * the agent rather than a thing to do with it.
 */
export function AgentMoreMenu({
  running,
  agentId,
  onRetire,
  label,
}: {
  running: boolean;
  agentId: string;
  onRetire: () => void;
  label: string;
}) {
  const { t } = useTranslation();
  const stop = useSetAgentPaused(agentId);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="size-9" aria-label={label}>
          <Ellipsis className="size-4" aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        {running && (
          <DropdownMenuItem
            onSelect={() =>
              stop.mutate(true, {
                onSuccess: () => toast.success(t("agents.stopped")),
                onError: (error) => toast.error(problemMessage(error, t)),
              })
            }
          >
            <Pause className="size-4" aria-hidden />
            {t("agents.stop")}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onSelect={onRetire}>
          <Archive className="size-4" aria-hidden />
          {t("agents.retire")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
