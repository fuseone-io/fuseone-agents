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
import { WorkItemRoles } from "@/features/work-items/work-item-roles";
import { WorkItemRuns } from "@/features/work-items/work-item-runs";

/**
 * A card's trail: every run that claimed this reference, and per role what
 * those runs asked people and cost (#204).
 *
 * It is a view of this reference only, and says so: an issue and the pull
 * request that answers it are two references until the platform links them.
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
        description={t("workItems.subtitle")}
      />
      {ref ? (
        <Trail reference={ref} />
      ) : (
        <EmptyState
          title={t("workItems.noRef")}
          hint={t("workItems.noRefHint")}
        />
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
          <Mono>{reference}</Mono>
        </AlertTitle>
        <AlertDescription>{t("workItems.scopeNote")}</AlertDescription>
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
            <WorkItemRuns runs={trail.runs} />
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
