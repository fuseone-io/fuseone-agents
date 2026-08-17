import { useTranslation } from "react-i18next";
import { useState } from "react";
import { CheckCheck } from "lucide-react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { LoadMore } from "@/components/shared/load-more";
import { PageHeader } from "@/components/shared/page-header";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { DecisionCard } from "@/features/approvals/decision-card";
import { DecisionPanel } from "@/features/approvals/decision-panel";
import { useApprovals } from "@/features/approvals/api";

export function ApprovalsPage() {
  const { t } = useTranslation();
  const {
    items,
    isLoading,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useApprovals();
  const [selectedRun, setSelectedRun] = useState<string>();

  // Derived, not synchronised: the state holds what somebody clicked, and the
  // fallback handles the rest. Deciding removes an item from the queue and the
  // panel follows on the next render, which is how the same action does not
  // get approved twice from a panel showing a decision already made.
  const selected = items.find((item) => item.runId === selectedRun) ?? items[0];

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.approvals}
        title={t("nav.approvals")}
        description={t("approvals.subtitle")}
      />

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          icon={<CheckCheck className="size-6" />}
          title={t("approvals.nothingWaiting")}
          hint={t("approvals.emptyHint")}
        />
      ) : (
        <div className="flex min-h-0 flex-1 gap-4">
          <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-auto">
            {items.map((item) => (
              <DecisionCard
                key={`${item.runId}-${item.atSeq}`}
                item={item}
                selected={item.runId === selected?.runId}
                onSelect={() => setSelectedRun(item.runId)}
              />
            ))}
            <LoadMore
              loaded={items.length}
              hasMore={hasNextPage}
              isLoading={isFetchingNextPage}
              onLoad={() => void fetchNextPage()}
            />
          </div>

          {selected && (
            <DecisionPanel
              key={`${selected.runId}-${selected.atSeq}`}
              item={selected}
              onDecided={() => setSelectedRun(undefined)}
            />
          )}
        </div>
      )}
    </>
  );
}
