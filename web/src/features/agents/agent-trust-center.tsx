import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  CircleDashed,
  type LucideIcon,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Panel } from "@/components/shared/panel";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { cn } from "@/lib/utils";
import { formatInstant, formatMicros } from "@/lib/format";
import type { Agent, AgentTrust } from "@/lib/api/client";

type TrustEvidence = AgentTrust["evidence"][number];
type TrustEvidenceStatus = TrustEvidence["status"];
type TrustStatus = AgentTrust["status"];

const STATUS_ICONS: Record<TrustEvidenceStatus, LucideIcon> = {
  good: CheckCircle2,
  bad: AlertTriangle,
  missing: CircleDashed,
  unknown: CircleDashed,
};

export function AgentTrustCenter({
  agent,
  trust,
  loading,
  error,
  onRetry,
}: {
  agent: Agent;
  trust?: AgentTrust;
  loading?: boolean;
  error?: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Panel
      title={t("agents.trustTitle")}
      action={trust && <StatusBadge status={trust.status} />}
    >
      {loading && <LoadingRows rows={2} />}
      {error ? <ErrorState error={error} onRetry={onRetry} /> : null}
      {!loading && !error && trust && <TrustBody agent={agent} trust={trust} />}
    </Panel>
  );
}

function TrustBody({ agent, trust }: { agent: Agent; trust: AgentTrust }) {
  return (
    <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
      <TrustSummary agent={agent} trust={trust} />

      <ol className="grid min-w-0 gap-2 sm:grid-cols-2">
        {trust.evidence.map((item) => (
          <li key={item.id} className="min-w-0">
            <EvidenceCard agent={agent} item={item} />
          </li>
        ))}
      </ol>
    </div>
  );
}

function TrustSummary({ agent, trust }: { agent: Agent; trust: AgentTrust }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-3">
      <p className="text-sm font-medium">
        {t(`agents.trustRecommend${titleID(trust.recommendation)}`)}
      </p>
      <p className="text-sm text-muted-foreground">
        {t(`agents.trustSummary${titleID(trust.summary)}`)}
      </p>
      <p className="text-xs leading-snug text-muted-foreground">
        {t("agents.trustWindow", {
          from: formatInstant(trust.window.from),
          until: formatInstant(trust.window.until),
        })}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="outline" asChild>
          <Link to={`/agents/${agent.agentId}/simulate`}>
            {t("agents.trustRunSimulation")}
          </Link>
        </Button>
        <Button size="sm" variant="outline" asChild>
          <Link to="/runtime">{t("agents.trustOpenRuntime")}</Link>
        </Button>
      </div>
    </div>
  );
}

function EvidenceCard({ agent, item }: { agent: Agent; item: TrustEvidence }) {
  const { t } = useTranslation();
  const Icon = STATUS_ICONS[item.status];
  return (
    <Link
      to={evidenceTarget(agent, item)}
      className="flex h-full min-w-0 gap-3 rounded-md border bg-background px-3 py-2 transition-colors hover:border-primary/50"
    >
      <Icon
        className={cn("mt-0.5 size-4 shrink-0", toneClass(item.status))}
        aria-hidden
      />
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">
            {t(`agents.trustEvidence${titleID(item.id)}`)}
          </span>
          <Badge variant="outline" className={toneClass(item.status)}>
            {t(statusKey(item.status))}
          </Badge>
        </span>
        <span className="mt-1 block text-2xs leading-snug text-muted-foreground">
          {t(`agents.trustCode${titleID(item.code)}`, trustValues(item.values))}
        </span>
      </span>
      <ArrowRight className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
    </Link>
  );
}

function StatusBadge({ status }: { status: TrustStatus }) {
  const { t } = useTranslation();
  return (
    <Badge variant="outline" className={toneClass(status)}>
      {t(statusKey(status))}
    </Badge>
  );
}

function evidenceTarget(agent: Agent, item: TrustEvidence) {
  switch (item.id) {
    case "simulation":
    case "version":
      return `/agents/${agent.agentId}/simulate`;
    case "decisions":
      return "/approvals";
    case "cost":
      return "/cost";
    case "policy":
      return "/runs";
    case "launch":
      return "/runtime";
    default:
      return "/runs";
  }
}

function trustValues(values?: Record<string, unknown>) {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(values ?? {})) {
    out[key] = key.endsWith("Micros") && typeof value === "number"
      ? formatMicros(value)
      : value;
  }
  return out;
}

function statusKey(status: TrustEvidenceStatus | TrustStatus) {
  return `agents.trustStatus${titleID(status)}`;
}

function toneClass(status: TrustEvidenceStatus | TrustStatus) {
  if (status === "good" || status === "ready") return "text-success";
  if (status === "bad" || status === "needs_review") return "text-danger";
  return "text-warning";
}

function titleID(value: string) {
  return value
    .split("_")
    .map((part) => `${part.slice(0, 1).toUpperCase()}${part.slice(1)}`)
    .join("");
}
