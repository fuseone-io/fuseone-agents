import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { TFunction } from "i18next";
import {
  Brain,
  Check,
  ChevronDown,
  CircleAlert,
  Clock,
  MessageSquareOff,
  Wrench,
} from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { reasonKey } from "@/features/agents/simulation-reason";
import {
  caseNeedsLook,
  stoppedByGate,
} from "@/features/agents/simulation-tally";
import { formatMicros } from "@/lib/format";
import { cn } from "@/lib/utils";
import type {
  SimulationAct,
  SimulationCase,
} from "@/features/agents/simulation-api";

/**
 * One situation, and what the agent would have done with it.
 *
 * The technical acts remain visible, but the first sentence says the useful
 * thing: answered, stopped, waited, or still running.
 */
export function CaseRow({
  index,
  entry,
  onCorrect,
}: {
  index: number;
  entry: SimulationCase;
  /** Absent when the case has no run to correct from. */
  onCorrect?: () => void;
}) {
  const { t } = useTranslation();
  const acts = entry.acted ?? [];
  const status = statusOf(entry, t);
  const StatusIcon = status.icon;
  const blocked = stoppedByGate(entry);

  return (
    <Collapsible>
      <div className="flex min-w-0 items-center gap-2 px-4 py-3">
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex min-w-0 flex-1 items-center gap-3 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span
              className={cn(
                "grid size-5 shrink-0 place-items-center rounded-full",
                status.className,
              )}
            >
              <StatusIcon className="size-3" aria-hidden />
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {t("simulation.situationNumber", { n: index })}
              </p>
              <p className="truncate text-xs text-muted-foreground">
                {status.subtitle}
              </p>
            </div>
            <span className={cn("shrink-0 text-xs", status.textClassName)}>
              {status.state}
            </span>
            <ChevronDown
              aria-hidden
              className="size-4 shrink-0 text-muted-foreground transition-transform [[data-state=open]_&]:rotate-180"
            />
          </button>
        </CollapsibleTrigger>

        {onCorrect && (
          <Button
            variant="ghost"
            size="sm"
            className="h-7 shrink-0 text-muted-foreground"
            onClick={onCorrect}
          >
            {t("correction.correct")}
          </Button>
        )}
      </div>

      <CollapsibleContent>
        <div className="flex min-w-0 flex-col gap-4 border-t bg-muted/50 px-4 py-4 pl-11">
          <section className="flex min-w-0 flex-col gap-2">
            <h3 className="text-2xs uppercase tracking-label text-muted-foreground">
              {t("simulation.whatItWouldDo")}
            </h3>
            {acts.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t("simulation.noActs")}
              </p>
            ) : (
              <ul className="flex flex-col gap-2">
                {acts.map((act, i) => (
                  <ActStep key={`${act.tool}-${i}`} act={act} />
                ))}
              </ul>
            )}
          </section>

          <section className="flex min-w-0 flex-col gap-2">
            <h3 className="text-2xs uppercase tracking-label text-muted-foreground">
              {t("simulation.answerWouldWrite")}
            </h3>
            <Outcome entry={entry} />
          </section>

          {(entry.unmet?.length ?? 0) > 0 ||
          entry.error ||
          entry.reason ||
          blocked ? (
            <div className="flex gap-2 rounded-lg bg-warning-surface px-3 py-2 text-sm text-warning">
              <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
              <span className="min-w-0 break-words">{warning(entry, t)}</span>
            </div>
          ) : null}

          <div className="flex flex-wrap items-center gap-2">
            {entry.runId && (
              <Link
                to={`/runs/${entry.runId}`}
                className="text-xs text-text-accent underline-offset-4 hover:underline"
              >
                {t("simulation.seeEverything")}
              </Link>
            )}
            <span className="ml-auto font-mono text-xs text-muted-foreground">
              {t("simulation.meta", {
                steps: entry.steps,
                calls: acts.length,
                cost: formatMicros(entry.cost.micros),
              })}
            </span>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ActStep({ act }: { act: SimulationAct }) {
  const { t } = useTranslation();
  const blocked = act.verdict === "block" || !act.reached;
  const Icon = blocked ? MessageSquareOff : act.reached ? Wrench : Brain;
  const tone = blocked
    ? "text-warning"
    : act.reached
      ? "text-text-accent"
      : "text-muted-foreground";

  return (
    <li className="flex min-w-0 items-start gap-3">
      <span className="grid size-5 shrink-0 place-items-center rounded-full border bg-card">
        <Icon className={cn("size-3", tone)} aria-hidden />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm">
          {blocked
            ? t("simulation.stepBlocked", { tool: act.tool })
            : t("simulation.stepWouldCall", { tool: act.tool })}
        </p>
        <Mono dim className="block truncate text-2xs">
          {act.tool}
          {" · "}
          {t(effectKey(act))}
          {act.policy || act.rule ? ` · ${act.policy ?? act.rule}` : ""}
        </Mono>
      </div>
      <span className="shrink-0 text-xs text-muted-foreground">
        {blocked ? t("simulation.heldBack") : t("simulation.dryCall")}
      </span>
    </li>
  );
}

function Outcome({ entry }: { entry: SimulationCase }) {
  const { t } = useTranslation();
  if (entry.outcome && entry.outcome.trim() !== "") {
    return (
      <blockquote className="max-w-3xl border-l-2 border-border-strong pl-3 text-sm leading-6 text-muted-foreground whitespace-pre-wrap">
        {entry.outcome}
      </blockquote>
    );
  }
  if (entry.outcomeState === "unavailable") {
    return (
      <p className="text-sm text-muted-foreground">
        {t("simulation.outcomeUnavailable")}
      </p>
    );
  }
  return (
    <p className="text-sm text-muted-foreground">
      {t("simulation.noAnswerYet")}
    </p>
  );
}

function statusOf(entry: SimulationCase, t: TFunction) {
  switch (entry.settled) {
    case "finished":
      if (caseNeedsLook(entry)) {
        return {
          icon: CircleAlert,
          className: "bg-warning-surface text-warning",
          textClassName: "text-warning",
          state: t("simulation.needsALook"),
          subtitle: t("simulation.gaveUpBeforeAnswering"),
        };
      }
      return {
        icon: Check,
        className: "bg-success-surface text-success",
        textClassName: "text-success",
        state: t("simulation.asExpected"),
        subtitle: t("simulation.answeredHeldBack"),
      };
    case "awaiting_approval":
      return {
        icon: CircleAlert,
        className: "bg-warning-surface text-warning",
        textClassName: "text-warning",
        state: t("simulation.needsALook"),
        subtitle: t("simulation.wouldAskSomeone"),
      };
    case "parked":
      return {
        icon: CircleAlert,
        className: "bg-warning-surface text-warning",
        textClassName: "text-warning",
        state: t("simulation.needsALook"),
        subtitle: t("simulation.gaveUpBeforeAnswering"),
      };
    default:
      return {
        icon: Clock,
        className: "bg-muted text-muted-foreground",
        textClassName: "text-muted-foreground",
        state: t("simulation.notRehearsed"),
        subtitle: t("simulation.waitingForResult"),
      };
  }
}

function warning(entry: SimulationCase, t: TFunction) {
  if (entry.error) return entry.error;
  if (entry.reason) {
    const key = reasonKey(entry.reason);
    return key ? t(key) : entry.reason;
  }
  if (stoppedByGate(entry)) return t("simulation.gateStoppedProposal");
  return t("simulation.unmetExpectations", {
    count: entry.unmet?.length ?? 0,
  });
}

const EFFECTS: Record<string, string> = {
  read: "simulation.effectRead",
  write: "simulation.effectWrite",
  destructive: "simulation.effectDestructive",
  financial: "simulation.effectFinancial",
};

function effectKey(act: SimulationAct): string {
  return EFFECTS[act.effect] ?? "simulation.effectUnknown";
}
