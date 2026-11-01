import { useTranslation } from "react-i18next";
import { Panel } from "@/components/shared/panel";
import { Badge } from "@/components/ui/badge";
import { formatDurationMs } from "@/lib/format";
import type { RuntimeEventDispatch } from "@/lib/api/client";

/**
 * What the event dispatcher has not reached yet (#228).
 *
 * The dispatcher reads only below the oldest open transaction, so a long
 * transaction anywhere in the database delays every dispatch while it is
 * open. This panel separates that wait from the dispatcher's own backlog and
 * says how long the holder has been open — or that the database role cannot
 * see it. Counts and ages only: no run, agent or transaction id.
 */
export function EventDispatchPanel({ dispatch }: { dispatch: RuntimeEventDispatch }) {
  const { t } = useTranslation();
  const degraded = dispatch.status === "degraded";
  return (
    <Panel title={t("runtime.eventDispatch")}>
      <Badge variant={degraded ? "destructive" : "secondary"}>
        {degraded ? t("runtime.eventDispatchDegraded") : t("runtime.eventDispatchHealthy")}
      </Badge>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <Count label={t("runtime.eventDispatchReady")} value={dispatch.ready} />
        <Count label={t("runtime.eventDispatchHeld")} value={dispatch.heldByXmin} />
      </div>
      <div className="mt-4 grid gap-1 text-xs text-muted-foreground">
        <p>
          {dispatch.ready + dispatch.heldByXmin === 0
            ? t("runtime.eventDispatchCaughtUp")
            : t("runtime.eventDispatchLag", { lag: seconds(dispatch.lagSeconds) })}
        </p>
        {dispatch.heldByXmin > 0 && <Holder dispatch={dispatch} />}
        <p>
          {t("runtime.eventDispatchThreshold", {
            threshold: seconds(dispatch.degradedAfterSeconds),
          })}
        </p>
      </div>
    </Panel>
  );
}

function Holder({ dispatch }: { dispatch: RuntimeEventDispatch }) {
  const { t } = useTranslation();
  if (!dispatch.xactAgeKnown) return <p>{t("runtime.eventDispatchXactHidden")}</p>;
  if (dispatch.xactAgeSeconds === undefined) return null;
  return <p>{t("runtime.eventDispatchXactAge", { age: seconds(dispatch.xactAgeSeconds) })}</p>;
}

function Count({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border bg-background px-3 py-2">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 font-mono text-2xl tabular-nums">{value}</div>
    </div>
  );
}

function seconds(value: number): string {
  return formatDurationMs(value * 1000);
}
