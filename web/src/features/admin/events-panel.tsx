import { useTranslation } from "react-i18next";
import { ScrollText } from "lucide-react";
import { Panel } from "@/components/shared/panel";
import { Mono } from "@/components/shared/mono";
import { LoadMore } from "@/components/shared/load-more";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { formatInstant } from "@/lib/format";
import { useAdminEvents } from "@/features/admin/api";

/**
 * What people changed about the platform.
 *
 * The run ledger says what agents did; this says what the rules were when they
 * did it. An auditor asking "why was this allowed" needs both.
 */
export function EventsPanel() {
  const { t } = useTranslation();
  const {
    items: events,
    isLoading,
    error,
    refetch,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useAdminEvents();

  return (
    <Panel
      title={t("admin.adminTrail")}
      action={
        <span className="text-xs text-muted-foreground">
          {t("admin.appendOnly")}
        </span>
      }
    >
      {isLoading ? (
        <LoadingRows rows={4} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : events.length === 0 ? (
        <EmptyState
          icon={<ScrollText className="size-6" />}
          title={t("policies.nothingChangedYetTitle")}
          hint={t("admin.eventsHint")}
        />
      ) : (
        <>
          <ul className="flex flex-col">
            {events.map((event, i) => (
              <li
                key={`${event.at}-${event.action}-${event.target}-${i}`}
                className="flex items-baseline gap-3 border-b py-2 last:border-b-0"
              >
                <Mono dim>{formatInstant(event.at)}</Mono>
                <Mono>{event.action}</Mono>
                <span className="min-w-0 flex-1 truncate text-sm">
                  {event.target}
                </span>
                <Mono dim>{event.principalId}</Mono>
              </li>
            ))}
          </ul>
          <LoadMore
            loaded={events.length}
            hasMore={hasNextPage}
            isLoading={isFetchingNextPage}
            onLoad={() => void fetchNextPage()}
          />
        </>
      )}
    </Panel>
  );
}
