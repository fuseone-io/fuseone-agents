import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Info } from "lucide-react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { LoadMore } from "@/components/shared/load-more";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { useWorkItemTrail } from "@/features/work-items/api";
import { WorkItemMeasuresPanel } from "@/features/work-items/work-item-measures-panel";
import { WorkItemRoles } from "@/features/work-items/work-item-roles";
import { WorkItemRuns } from "@/features/work-items/work-item-runs";

/**
 * A card's trail: every run that claimed the card or a pull request linked to
 * it, and per role what those runs asked people and cost (#204).
 *
 * With no card asked for, the page is the weekly measures per role instead.
 *
 * Opened by a linked pull request, it is the card's trail, headed by the card
 * and saying which pull request was asked for.
 */
export function WorkItemPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const ref = params.get("ref")?.trim() ?? "";

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS["work-items"]}
        title={t("workItems.title")}
        description={t(
          ref ? "workItems.subtitle" : "workItems.measures.subtitle",
        )}
      />
      {ref ? (
        <Trail reference={ref} />
      ) : (
        <WorkItemMeasuresPanel />
      )}
    </>
  );
}

function Trail({ reference }: { reference: string }) {
  const { t } = useTranslation();
  const trail = useWorkItemTrail(reference);

  return (
    <>
      <Alert>
        <Info aria-hidden />
        <AlertTitle>
          <Mono>{trail.card ?? reference}</Mono>
        </AlertTitle>
        <AlertDescription>
          {trail.resolvedFrom ? (
            <p>{t("workItems.resolvedNote", { from: trail.resolvedFrom })}</p>
          ) : null}
          <p>{t("workItems.scopeNote")}</p>
        </AlertDescription>
      </Alert>
      {trail.isLoading ? (
        <LoadingRows />
      ) : trail.error ? (
        <ErrorState error={trail.error} onRetry={() => void trail.refetch()} />
      ) : trail.runs.length === 0 ? (
        <EmptyState
          title={t("workItems.empty")}
          hint={t("workItems.emptyHint")}
        />
      ) : (
        <>
          <Panel title={t("workItems.roles")} flush>
            <WorkItemRoles roles={trail.roles} />
          </Panel>
          <Panel title={t("workItems.timeline")} flush>
            <WorkItemRuns runs={trail.runs} card={trail.card ?? reference} />
            <div className="px-4 pb-3">
              <LoadMore
                loaded={trail.runs.length}
                hasMore={trail.hasNextPage}
                isLoading={trail.isFetchingNextPage}
                onLoad={() => void trail.fetchNextPage()}
              />
            </div>
          </Panel>
        </>
      )}
    </>
  );
}
