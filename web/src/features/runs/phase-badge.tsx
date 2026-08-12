import { Badge } from "@/components/ui/badge";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { StateDot } from "@/components/shared/state-dot";
import { STATE_TEXT, stateOfPhase } from "@/lib/agent-state";
import type { Phase } from "@/lib/api/client";

// Colour never carries the meaning on its own: every phase reads as a word
// first, so the state survives a monochrome print, a colour-blind reader and a
// screenshot pasted into a ticket. The dot is reinforcement, not the message.
export const PHASE_LABELS: Record<Phase, string> = {
  unstarted: "runs.phaseUnstarted",
  running: "runs.phaseRunning",
  awaiting_approval: "runs.phaseAwaitingApproval",
  awaiting_tool: "runs.phaseAwaitingTool",
  parked: "runs.phaseParked",
  finished: "runs.phaseFinished",
};

export function PhaseBadge({ phase }: { phase: Phase }) {
  const state = stateOfPhase(phase);

  const { t } = useTranslation();

  return (
    <Badge
      variant="outline"
      className={cn("gap-1.5 font-normal", STATE_TEXT[state])}
    >
      <StateDot state={state} />
      {t(PHASE_LABELS[phase])}
    </Badge>
  );
}
