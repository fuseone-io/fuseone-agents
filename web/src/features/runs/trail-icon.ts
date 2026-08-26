import {
  CircleDollarSign,
  Flag,
  Hand,
  Play,
  PlayCircle,
  Scale,
  ShieldAlert,
  ShieldCheck,
  ShieldX,
  CircleSlash,
  Sparkles,
  TriangleAlert,
  UserRoundCheck,
  Undo2,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import { verdictOf } from "@/features/runs/step-verb";
import type { Step, StepKind } from "@/lib/api/client";

/**
 * The tile that opens each event in the trail.
 *
 * Its colour encodes the nature of the event, never the event itself — the
 * title beside it always says what happened in words. Neutral is the default
 * because most of a run is routine, and a trail where everything is coloured
 * says nothing about where to look.
 */
export type TileTone = "neutral" | "allow" | "escalate" | "block" | "live";

export const TILE: Record<TileTone, string> = {
  neutral: "border-border bg-card text-muted-foreground",
  allow: "border-success bg-success-surface text-success",
  escalate: "border-warning bg-warning-surface text-warning",
  block: "border-danger bg-danger-surface text-danger",
  live: "border-primary bg-surface-accent text-text-accent motion-safe:animate-pulse",
};

const ICONS: Record<StepKind, LucideIcon> = {
  run_started: Play,
  planned: Sparkles,
  gate_decided: ShieldCheck,
  budget_reserved: CircleDollarSign,
  tool_called: Wrench,
  tool_returned: Wrench,
  budget_reconciled: Scale,
  approval_requested: Hand,
  approval_decided: UserRoundCheck,
  resumed: PlayCircle,
  abandoned: CircleSlash,
  compensated: Undo2,
  failed: TriangleAlert,
  parked: ShieldAlert,
  run_finished: Flag,
};

const VERDICT_TONE: Record<string, TileTone> = {
  allow: "allow",
  constrain: "escalate",
  require_approval: "escalate",
  block: "block",
  duplicate: "neutral",
};

const VERDICT_ICON: Record<string, LucideIcon> = {
  allow: ShieldCheck,
  constrain: ShieldAlert,
  require_approval: ShieldAlert,
  block: ShieldX,
  duplicate: CircleSlash,
};

export function tileOf(
  step: Step,
  live: boolean,
): { icon: LucideIcon; tone: TileTone } {
  const verdict = verdictOf(step);
  if (verdict) {
    return {
      icon: VERDICT_ICON[verdict] ?? ShieldCheck,
      tone: live ? "live" : (VERDICT_TONE[verdict] ?? "neutral"),
    };
  }
  const icon = ICONS[step.kind];
  if (live) return { icon, tone: "live" };
  switch (step.kind) {
    case "approval_requested":
      return { icon, tone: "escalate" };
    case "abandoned":
    case "failed":
    case "parked":
      return { icon, tone: "block" };
    case "run_finished":
      return { icon, tone: "allow" };
    default:
      return { icon, tone: "neutral" };
  }
}
