import { cn } from "@/lib/utils";
import { STATE_DOT, type AgentState } from "@/lib/agent-state";

/**
 * The 7px state dot.
 *
 * Decorative by contract: it always sits beside a word that carries the same
 * meaning, so it is hidden from assistive technology rather than announced as
 * a colour nobody can act on.
 */
export function StateDot({ state, className }: { state: AgentState; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn("size-[7px] shrink-0 rounded-pill", STATE_DOT[state], className)}
    />
  );
}
