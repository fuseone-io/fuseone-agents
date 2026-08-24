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
import {
  agentTrustModel,
  type TrustEvidence,
  type TrustEvidenceStatus,
  type TrustStatus,
} from "@/features/agents/agent-trust-model";
import { cn } from "@/lib/utils";
import type { Agent } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type RegressionCase = components["schemas"]["RegressionCase"];
const STATUS_ICONS: Record<TrustEvidenceStatus, LucideIcon> = {
  good: CheckCircle2,
  bad: AlertTriangle,
  missing: CircleDashed,
  unknown: CircleDashed,
};

export function AgentTrustCenter({
  agent,
  regressions,
  regressionsLoading,
  regressionsError,
}: {
  agent: Agent;
  regressions?: RegressionCase[];
  regressionsLoading?: boolean;
  regressionsError?: unknown;
}) {
  const { t } = useTranslation();
  const model = agentTrustModel({
    agent,
    regressions,
    regressionsLoading,
    regressionsError,
  });

  return (
    <Panel
      title={t("agents.trustTitle")}
      action={<StatusBadge status={model.status} />}
    >
      <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
        <TrustSummary agent={agent} model={model} />

        <ol className="grid min-w-0 gap-2 sm:grid-cols-2">
          {model.evidence.map((item) => (
            <li key={item.id} className="min-w-0">
              <EvidenceCard item={item} />
            </li>
          ))}
        </ol>
      </div>
    </Panel>
  );
}

function TrustSummary({
  agent,
  model,
}: {
  agent: Agent;
  model: ReturnType<typeof agentTrustModel>;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-3">
      <p className="text-sm font-medium">{t(model.recommendationKey)}</p>
      <p className="text-sm text-muted-foreground">{t(model.summaryKey)}</p>
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

function EvidenceCard({ item }: { item: TrustEvidence }) {
  const { t } = useTranslation();
  const Icon = STATUS_ICONS[item.status];
  return (
    <Link
      to={item.to}
      className="flex h-full min-w-0 gap-3 rounded-md border bg-background px-3 py-2 transition-colors hover:border-primary/50"
    >
      <Icon
        className={cn("mt-0.5 size-4 shrink-0", toneClass(item.status))}
        aria-hidden
      />
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">
            {t(item.titleKey)}
          </span>
          <Badge variant="outline" className={toneClass(item.status)}>
            {t(statusKey(item.status))}
          </Badge>
        </span>
        <span className="mt-1 block text-2xs leading-snug text-muted-foreground">
          {t(item.bodyKey, item.bodyValues)}
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
