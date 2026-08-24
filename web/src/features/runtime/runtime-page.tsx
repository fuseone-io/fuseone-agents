import { AlertTriangle } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StateDot } from "@/components/shared/state-dot";
import { formatRelative } from "@/lib/format";
import { stateOfPhase } from "@/lib/agent-state";
import { PHASE_LABELS } from "@/features/runs/phase-badge";
import { useRuntimeHealth } from "@/features/runtime/api";
import { failureLabel } from "@/features/runtime/failure-labels";
import { ToolFailuresPanel } from "@/features/runtime/tool-failures-panel";
import type { Phase, RuntimeFailureBucket, RuntimeHealth } from "@/lib/api/client";

const WINDOW_MS = 24 * 60 * 60 * 1000;
const PHASES: Phase[] = [
  "running",
  "awaiting_tool",
  "awaiting_approval",
  "parked",
  "failed",
  "finished",
];

export function RuntimePage() {
  const { t } = useTranslation();
  const [since] = useState(() => new Date(Date.now() - WINDOW_MS).toISOString());
  const health = useRuntimeHealth(since);

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.runtime}
        title={t("runtime.title")}
        description={t("runtime.subtitle")}
      />

      {health.isLoading ? (
        <LoadingRows rows={6} />
      ) : health.error ? (
        <ErrorState error={health.error} onRetry={() => void health.refetch()} />
      ) : health.data ? (
        <RuntimeBody health={health.data} />
      ) : null}
    </>
  );
}

function RuntimeBody({ health }: { health: RuntimeHealth }) {
  return (
    <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <QueuePanel health={health} />
      <ProviderFailures failures={health.failures} />
      <ToolFailuresPanel failures={health.toolFailures} />
      <PhasePanel health={health} />
    </div>
  );
}

function QueuePanel({ health }: { health: RuntimeHealth }) {
  const { t } = useTranslation();
  const q = health.queue;
  return (
    <Panel title={t("runtime.queue")}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Metric label={t("runtime.ready")} value={q.ready} tone="ok" />
        <Metric label={t("runtime.leased")} value={q.leased} tone="neutral" />
        <Metric label={t("runtime.backingOff")} value={q.backingOff} tone="warn" />
        <Metric label={t("runtime.expiredLeases")} value={q.expiredLeases} tone="bad" />
      </div>
      <p className="mt-4 text-xs text-muted-foreground">
        {q.oldestReadyAt
          ? t("runtime.oldestReady", { seen: formatRelative(q.oldestReadyAt) })
          : t("runtime.noneReady")}
      </p>
    </Panel>
  );
}

function ProviderFailures({ failures }: { failures: RuntimeFailureBucket[] }) {
  const { t } = useTranslation();
  return (
    <Panel title={t("runtime.providerFailures")} flush>
      {failures.length === 0 ? (
        <div className="p-4 text-sm text-muted-foreground">
          {t("runtime.noProviderFailures")}
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead>{t("runtime.cause")}</TableHead>
              <TableHead>{t("runtime.provider")}</TableHead>
              <TableHead className="text-right">{t("runtime.runs")}</TableHead>
              <TableHead className="text-right">{t("runtime.lastSeen")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {failures.map((failure) => (
              <TableRow key={keyOf(failure)}>
                <TableCell>
                  <div className="flex min-w-0 items-center gap-2">
                    <AlertTriangle className="size-4 shrink-0 text-warning" />
                    <div className="min-w-0">
                      <div className="truncate font-medium">
                        {t(failureLabel(failure.code))}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {failure.status
                          ? t("runtime.status", { status: failure.status })
                          : t("runtime.noStatus")}
                        {" · "}
                        {failure.retryable
                          ? t("runtime.retryable")
                          : t("runtime.notRetryable")}
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell>{failure.provider || "—"}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {failure.runs}
                </TableCell>
                <TableCell className="text-right text-muted-foreground">
                  {formatRelative(failure.lastAt)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Panel>
  );
}

function PhasePanel({ health }: { health: RuntimeHealth }) {
  const { t } = useTranslation();
  return (
    <Panel title={t("runtime.phases")} className="xl:col-span-2">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {PHASES.map((phase) => (
          <div
            key={phase}
            className="flex min-w-0 items-center justify-between gap-3 rounded-md border bg-background px-3 py-2"
          >
            <span className="flex min-w-0 items-center gap-2">
              <StateDot state={stateOfPhase(phase)} />
              <span className="truncate text-sm">{t(PHASE_LABELS[phase])}</span>
            </span>
            <span className="font-mono text-sm tabular-nums">
              {health.byPhase[phase] ?? 0}
            </span>
          </div>
        ))}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        {t("runtime.phaseWindowHint")}
      </p>
    </Panel>
  );
}

function Metric({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "ok" | "neutral" | "warn" | "bad";
}) {
  const color =
    tone === "bad"
      ? "text-danger"
      : tone === "warn"
        ? "text-warning"
        : tone === "ok"
          ? "text-success"
          : "text-foreground";
  return (
    <div className="rounded-md border bg-background px-3 py-2">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className={`mt-1 font-mono text-2xl tabular-nums ${color}`}>
        {value}
      </div>
    </div>
  );
}

function keyOf(failure: RuntimeFailureBucket): string {
  return [
    failure.code,
    failure.provider ?? "",
    String(failure.status ?? 0),
    failure.retryable ? "1" : "0",
  ].join(":");
}
