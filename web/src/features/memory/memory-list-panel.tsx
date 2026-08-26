import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { UseQueryResult } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { Panel } from "@/components/shared/panel";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MemoryAssertionCard } from "@/features/memory/memory-assertion-card";
import { MemoryAssertionRow } from "@/features/memory/memory-assertion-row";
import type {
  MemoryAssertion,
  MemoryFilters,
  MemoryStatusFilter,
} from "@/features/memory/api";
import { MEMORY_STATUS_LABELS } from "@/features/memory/memory-status";
import { useVisibleItems } from "@/hooks/use-visible-items";

const PAGE_SIZE = 8;
const STATUSES: MemoryStatusFilter[] = [
  "active",
  "disabled",
  "expired",
  "source_erased",
  "all",
];

export function MemoryListPanel({
  filters,
  onFilters,
  query,
  canDisable,
  onCorrect,
  onDisable,
}: {
  filters: MemoryFilters;
  onFilters: (filters: MemoryFilters) => void;
  query: UseQueryResult<{ items: MemoryAssertion[] }, Error>;
  canDisable: boolean;
  onCorrect?: (assertion: MemoryAssertion) => void;
  onDisable: (assertion: MemoryAssertion) => void;
}) {
  const { t } = useTranslation();
  const items = query.data?.items ?? [];
  const page = useVisibleItems(items, PAGE_SIZE);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const selected =
    page.visible.find((assertion) => assertion.id === selectedID) ??
    page.visible[0] ??
    null;

  return (
    <Panel title={t("memory.assertions")}>
      <MemoryFiltersBar filters={filters} onFilters={onFilters} />
      {query.isLoading ? (
        <LoadingRows rows={5} />
      ) : query.error ? (
        <ErrorState error={query.error} onRetry={() => void query.refetch()} />
      ) : page.visible.length === 0 ? (
        <EmptyState
          icon={<Search className="size-6" />}
          title={t("memory.emptyTitle")}
          hint={t("memory.emptyHint")}
        />
      ) : (
        <>
          <div className="grid min-w-0 gap-4 2xl:grid-cols-[minmax(0,1fr)_minmax(360px,440px)]">
            <div className="grid min-w-0 content-start gap-2">
              {page.visible.map((assertion) => (
                <MemoryAssertionRow
                  key={assertion.id}
                  assertion={assertion}
                  selected={assertion.id === selected?.id}
                  onSelect={() => setSelectedID(assertion.id)}
                />
              ))}
            </div>
            {selected && (
              <div className="min-w-0 2xl:sticky 2xl:top-4 2xl:self-start">
                <MemoryAssertionCard
                  assertion={selected}
                  canDisable={canDisable}
                  onCorrect={onCorrect}
                  onDisable={onDisable}
                />
              </div>
            )}
          </div>
          <LoadMore
            loaded={page.loaded}
            total={page.total}
            hasMore={page.hasMore}
            isLoading={false}
            onLoad={page.loadMore}
          />
        </>
      )}
    </Panel>
  );
}

function MemoryFiltersBar({
  filters,
  onFilters,
}: {
  filters: MemoryFilters;
  onFilters: (filters: MemoryFilters) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mb-4 grid min-w-0 gap-2 md:grid-cols-[minmax(0,1fr)_180px_180px]">
      <Input
        type="search"
        aria-label={t("memory.search")}
        placeholder={t("memory.searchPlaceholder")}
        value={filters.search}
        onChange={(event) => onFilters({ ...filters, search: event.target.value })}
      />
      <Input
        aria-label={t("memory.agentFilter")}
        placeholder={t("memory.agentFilterPlaceholder")}
        value={filters.agentId}
        onChange={(event) => onFilters({ ...filters, agentId: event.target.value })}
      />
      <Select
        value={filters.status}
        onValueChange={(status) =>
          onFilters({ ...filters, status: status as MemoryStatusFilter })
        }
      >
        <SelectTrigger aria-label={t("memory.statusFilter")}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {STATUSES.map((status) => (
            <SelectItem key={status} value={status}>
              {t(statusLabel(status))}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function statusLabel(status: MemoryStatusFilter): string {
  return status === "all" ? "memory.status.all" : MEMORY_STATUS_LABELS[status];
}
