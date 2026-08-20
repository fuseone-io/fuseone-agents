import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { SimulationReadiness } from "@/features/agents/simulation-readiness-state";

export function SimulationReadinessNotice({
  readiness,
  agentId,
  onRetry,
}: {
  readiness: SimulationReadiness;
  agentId: string;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-wrap items-start gap-3 rounded-lg border border-warning bg-warning-surface p-3 text-sm">
      <AlertTriangle
        className="mt-0.5 size-4 shrink-0 text-warning"
        aria-hidden
      />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{readiness.title}</p>
        <p className="text-muted-foreground">{readiness.body}</p>
      </div>
      {readiness.canRetry && onRetry && (
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          {t("common.retry")}
        </Button>
      )}
      {readiness.canOpenAgent && (
        <Button asChild variant="outline" size="sm">
          <Link to={`/agents/${agentId}`}>{t("agents.openAgent")}</Link>
        </Button>
      )}
    </div>
  );
}
