import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { useDecisions } from "@/features/overview/api";
import { formatTime } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { RecordedDecision, Verdict } from "@/lib/api/client";

// Five rows of h-8. Fixed only once there are more than five, so a quiet
// morning shows three rows rather than three rows and a hole.
const VISIBLE = 5;

const VERDICT: Record<Verdict, { verb: string; className: string }> = {
  allow: { verb: "permitiu", className: "text-success" },
  constrain: { verb: "restringiu", className: "text-warning" },
  require_approval: { verb: "escalou", className: "text-warning" },
  block: { verb: "bloqueou", className: "text-danger" },
};

/**
 * What the Gate has been deciding.
 *
 * The one panel that says whether the installation's rules are doing anything.
 * A feed of nothing but "permitiu" means the policy is not engaging; a run of
 * escalations on one rule means it is engaging too much. Neither is visible
 * from inside a single run, which is the only place this used to be readable.
 */
export function DecisionsFeed({ since }: { since: string }) {
  const { t } = useTranslation();
  const { data, isLoading, error } = useDecisions(since);
  const items = data?.items ?? [];

  return (
    <section className="flex flex-col gap-2 rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium">{t("overview.decisions")}</h2>
        <span
          aria-hidden
          className="size-1.5 rounded-pill bg-success motion-safe:animate-pulse"
        />
        <span className="text-xs text-muted-foreground">
          {t("overview.live")}
        </span>
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-2 py-1">
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className="h-6 rounded" />
          ))}
        </div>
      ) : error ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t("overview.decisionsUnreadable")}
        </p>
      ) : items.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          {t("overview.noDecisions")}
        </p>
      ) : (
        // The feed is live. Left to grow, every decision the Gate records
        // would push the runs table further down under somebody reading it.
        <ScrollArea
          type="auto"
          className={cn(items.length > VISIBLE && "h-40")}
        >
          <ol className="flex flex-col pr-2.5">
            {items.map((decision) => (
              <Row
                key={`${decision.runId}-${decision.seq}`}
                decision={decision}
              />
            ))}
          </ol>
        </ScrollArea>
      )}
    </section>
  );
}

function Row({ decision }: { decision: RecordedDecision }) {
  const verdict = VERDICT[decision.verdict];

  return (
    <li>
      <Link
        to={`/runs/${decision.runId}`}
        className="flex h-8 items-center gap-2 rounded-md px-1 hover:bg-muted"
      >
        <Mono dim className="text-2xs">
          {formatTime(decision.at)}
        </Mono>
        <span className={cn("text-xs font-medium", verdict.className)}>
          {verdict.verb}
        </span>
        <Mono className="truncate">{decision.tool}</Mono>
        {/* Whose agent it was. Without it the feed says something was blocked
            and leaves the reader to find out for whom. */}
        <span className="shrink-0 truncate text-xs text-muted-foreground">
          {decision.agentId}
        </span>
        {/* The rule, never a category: t("overview.refusedByPolicy") tells a reader
            nothing about what to change. */}
        {decision.rule && decision.rule !== "passed" && (
          <Mono dim className="ml-auto shrink-0 text-2xs">
            {decision.rule}
          </Mono>
        )}
      </Link>
    </li>
  );
}
