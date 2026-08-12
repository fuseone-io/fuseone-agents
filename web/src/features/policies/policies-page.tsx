import { useTranslation } from "react-i18next";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Plus, Scale } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { PoliciesTable } from "@/features/policies/policies-table";
import { tallyOf } from "@/features/policies/policy-tally";
import { usePolicies } from "@/features/policies/api";
import { sinceFor } from "@/features/runs/runs-filters";

/**
 * Every rule the installation runs under.
 *
 * The figures at the top count three states rather than two: off is a
 * decision, watching is a rule that runs and changes nothing, and in force is
 * the only one that stops anything.
 */
export function PoliciesPage() {
  const { t } = useTranslation();
  const [period] = useState("7");
  const since = useMemo(() => sinceFor(period), [period]);
  const { data, isLoading, error, refetch } = usePolicies(since);

  const policies = data?.items ?? [];
  const tally = tallyOf(policies);

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.policies}
        title={t("nav.policies")}
        description={t("policies.subtitle")}
      >
        <Button size="sm" asChild>
          <Link to="/policies/new">
            <Plus className="size-4" aria-hidden />
            {t("policies.newPolicy")}
          </Link>
        </Button>
      </PageHeader>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Figure
          label="Impondo"
          value={String(tally.enforcing)}
          note="param alguma coisa"
        />
        <Figure
          label="Monitorando"
          value={String(tally.monitoring)}
          note={t("policies.evaluatedNotObeyed")}
        />
        <Figure
          label={t("policies.denials")}
          value={String(tally.denied)}
          note={t("policies.last7Days")}
        />
        <Figure
          label={t("policies.escalations")}
          value={String(tally.escalated)}
          note={t("policies.last7Days")}
        />
      </div>

      <Panel
        title={t("policies.rules")}
        action={
          data?.policyHash ? (
            // The set as a whole has a name, and every decision records it.
            // Showing it is how somebody checks that a worker picked up a
            // change without reading a log.
            <Mono dim className="text-2xs">
              {t("policies.setNamed", { hash: data.policyHash.slice(0, 12) })}
            </Mono>
          ) : undefined
        }
        flush
      >
        {isLoading ? (
          <div className="p-4">
            <LoadingRows rows={5} />
          </div>
        ) : error ? (
          <div className="p-4">
            <ErrorState error={error} onRetry={() => void refetch()} />
          </div>
        ) : policies.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={<Scale className="size-6" />}
              title={t("policies.noneWritten")}
              hint={t("policies.emptyHint")}
            />
          </div>
        ) : (
          <PoliciesTable policies={policies} />
        )}
      </Panel>
    </>
  );
}

function Figure({
  label,
  value,
  note,
}: {
  label: string;
  value: string;
  note: string;
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="text-2xs uppercase tracking-label text-muted-foreground">
        {label}
      </div>
      <div className="mt-1.5 font-mono text-[22px]/7 font-medium tabular-nums">
        {value}
      </div>
      <div className="mt-0.5 text-xs text-muted-foreground">{note}</div>
    </div>
  );
}
