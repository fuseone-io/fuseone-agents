import { AlertCircle, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { formatInstant } from "@/lib/format";
import type { ApprovalEvidenceDetail } from "@/lib/api/client";

export function ApprovalEvidenceView({
  evidence,
  loading,
  failed,
  onRetry,
}: {
  evidence: ApprovalEvidenceDetail | undefined;
  loading: boolean;
  failed: boolean;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="space-y-2 border-t border-border-subtle pt-3" aria-busy>
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-16 w-full" />
      </div>
    );
  }
  if (failed) {
    return (
      <Alert variant="destructive">
        <AlertCircle aria-hidden />
        <AlertTitle>{t("approvals.evidenceUnavailable")}</AlertTitle>
        <AlertDescription className="grid gap-2">
          {t("approvals.evidenceUnavailableHint")}
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RefreshCw aria-hidden className="size-4" />
            {t("common.retry")}
          </Button>
        </AlertDescription>
      </Alert>
    );
  }
  if (!evidence || evidence.kind === "none" || !evidence.gravitee) return null;

  const item = evidence.gravitee;
  return (
    <section className="border-t border-border-subtle pt-3">
      <h3 className="text-2xs uppercase tracking-label text-muted-foreground">
        {t("approvals.inspectedSubscription")}
      </h3>
      <dl className="mt-2 grid gap-2 text-sm">
        <EvidenceRow label={t("approvals.application")} value={item.application.name} />
        <EvidenceRow label={t("approvals.owner")} value={item.application.primaryOwnerEmail} />
        <EvidenceRow label="API" value={item.api.name} />
        <EvidenceRow label={t("approvals.plan")} value={item.plan.name} />
        <EvidenceRow
          label={t("approvals.expiration")}
          value={
            item.requestedExpiration
              ? formatInstant(item.requestedExpiration)
              : t("approvals.noExpiration")
          }
        />
        <EvidenceRow
          label={t("approvals.inspectedAt")}
          value={formatInstant(item.remoteUpdatedAt)}
        />
      </dl>
    </section>
  );
}

function EvidenceRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-2">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="break-words text-xs">{value}</dd>
    </div>
  );
}
