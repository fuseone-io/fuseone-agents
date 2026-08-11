import type { Phase } from "@/lib/api/client";

/**
 * The five run states the design system gives a colour to.
 *
 * The console's phases are finer grained than the palette on purpose: an
 * operator scanning a list wants to know whether something needs them, not
 * which internal phase the interpreter is in. `awaiting_tool` and `running`
 * are both simply running to that reader; the exact phase is still on the run
 * itself.
 */
export type AgentState = "draft" | "running" | "waiting" | "blocked" | "done";

const BY_PHASE: Record<Phase, AgentState> = {
  unstarted: "draft",
  running: "running",
  awaiting_tool: "running",
  awaiting_approval: "waiting",
  parked: "blocked",
  finished: "done",
};

export function stateOfPhase(phase: Phase): AgentState {
  return BY_PHASE[phase];
}

/**
 * Colour is never the message — the design system requires a dot *and* a
 * label — so these classes only ever decorate text that already says it.
 * Written as literals because Tailwind reads the source, not the runtime.
 */
export const STATE_DOT: Record<AgentState, string> = {
  draft: "bg-agent-draft",
  running: "bg-agent-running",
  waiting: "bg-agent-waiting",
  blocked: "bg-agent-blocked",
  done: "bg-agent-done",
};

export const STATE_TEXT: Record<AgentState, string> = {
  draft: "text-muted-foreground",
  running: "text-info",
  waiting: "text-warning",
  blocked: "text-danger",
  done: "text-success",
};

export const STATE_SURFACE: Record<AgentState, string> = {
  draft: "bg-muted",
  running: "bg-info-surface",
  waiting: "bg-warning-surface",
  blocked: "bg-danger-surface",
  done: "bg-success-surface",
};

/**
 * The state an agent reads as, from the phase of its most recent run.
 *
 * Never having run is `draft` — a different thing from having run and
 * finished, which is why the activity is absent rather than zeroed.
 */
export function stateOfAgent(lastPhase: Phase | undefined): AgentState {
  return lastPhase ? stateOfPhase(lastPhase) : "draft";
}
