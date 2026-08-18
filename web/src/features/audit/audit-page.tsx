import { useTranslation } from "react-i18next";
import { useDeferredValue, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, ScrollText } from "lucide-react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Toolbar } from "@/components/shared/toolbar";
import { Button } from "@/components/ui/button";
import {
  FilterSelect,
  type FilterOption,
} from "@/components/shared/filter-select";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { AuditTable } from "@/features/audit/audit-table";
import { IntegrityBanner } from "@/features/audit/integrity-banner";
import { useAuditPage, type AuditFilters } from "@/features/audit/api";
import { sinceFor } from "@/features/runs/runs-filters";

const SOURCES: FilterOption[] = [
  { value: "all", label: "audit.bothRecords" },
  { value: "ledger", label: "audit.ledgerDecisions" },
  { value: "admin", label: "audit.adminChanges" },
];

const PERIODS: FilterOption[] = [
  { value: "1", label: "audit.last24h" },
  { value: "7", label: "audit.last7d" },
  { value: "30", label: "audit.last30d" },
  { value: "all", label: "audit.sinceBeginning" },
];

interface AuditPageState {
  filterKey: string;
  index: number;
  cursors: Array<string | undefined>;
}

function emptyPageState(filterKey: string): AuditPageState {
  return { filterKey, index: 0, cursors: [undefined] };
}

/**
 * Everything that happened, from both records that keep it.
 *
 * The two are merged because a person asking "what happened" does not care
 * which table it landed in — and separated by their seal, because only one of
 * them can prove it was not altered.
 */
export function AuditPage() {
  const { t } = useTranslation();
  const [source, setSource] = useState("all");
  const [period, setPeriod] = useState("7");
  const [actor, setActor] = useState("");
  const query = useDeferredValue(actor.trim());

  const since = useMemo(() => sinceFor(period), [period]);
  const filterKey = `${source}\u0000${since ?? ""}\u0000${query}`;
  const [page, setPage] = useState<AuditPageState>(() =>
    emptyPageState(filterKey),
  );
  const activePage =
    page.filterKey === filterKey ? page : emptyPageState(filterKey);
  const filters = useMemo<AuditFilters>(
    () => ({
      since,
      actor: query || undefined,
      source: source === "all" ? undefined : (source as "ledger" | "admin"),
    }),
    [query, since, source],
  );
  const cursor = activePage.cursors[activePage.index];
  const { data, isLoading, error, refetch } = useAuditPage(filters, cursor);
  const entries = data?.items ?? [];
  const nextCursor = data?.nextCursor ?? null;

  function nextPage() {
    if (!nextCursor) return;
    setPage((current) => {
      const base =
        current.filterKey === filterKey ? current : emptyPageState(filterKey);
      return {
        filterKey,
        index: base.index + 1,
        cursors: [...base.cursors.slice(0, base.index + 1), nextCursor],
      };
    });
  }

  function previousPage() {
    setPage((current) => {
      const base =
        current.filterKey === filterKey ? current : emptyPageState(filterKey);
      return { ...base, index: Math.max(0, base.index - 1) };
    });
  }

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.audit}
        title={t("nav.audit")}
        description={t("audit.subtitle")}
      />

      {!isLoading && !error && <IntegrityBanner entries={entries} />}

      <Toolbar
        placeholder={t("audit.whoActed")}
        value={actor}
        onChange={setActor}
      >
        <FilterSelect
          label={t("audit.record")}
          value={source}
          options={SOURCES}
          onChange={setSource}
          width={230}
        />
        <FilterSelect
          label={t("audit.period")}
          value={period}
          options={PERIODS}
          onChange={setPeriod}
          width={180}
        />
      </Toolbar>

      <Panel
        title={t("runs.trail")}
        action={
          <span className="text-xs text-muted-foreground">
            {t("admin.appendOnly")}
          </span>
        }
        flush
      >
        {isLoading ? (
          <div className="p-4">
            <LoadingRows rows={8} />
          </div>
        ) : error ? (
          <div className="p-4">
            <ErrorState error={error} onRetry={() => void refetch()} />
          </div>
        ) : entries.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={<ScrollText className="size-6" />}
              title={t("audit.nothingInPeriod")}
              hint={t("audit.emptyHint")}
            />
          </div>
        ) : (
          <>
            <AuditTable entries={entries} />
            <AuditPager
              page={activePage.index + 1}
              rows={entries.length}
              hasPrevious={activePage.index > 0}
              hasNext={Boolean(nextCursor)}
              onPrevious={previousPage}
              onNext={nextPage}
            />
          </>
        )}
      </Panel>
    </>
  );
}

function AuditPager({
  page,
  rows,
  hasPrevious,
  hasNext,
  onPrevious,
  onNext,
}: {
  page: number;
  rows: number;
  hasPrevious: boolean;
  hasNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 px-4 pb-3 pt-2">
      <p className="text-xs text-muted-foreground tabular-nums">
        {t("audit.pageRows", { page, rows })}
      </p>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={onPrevious}
          disabled={!hasPrevious}
        >
          <ChevronLeft className="size-4" aria-hidden />
          {t("common.previous")}
        </Button>
        <Button variant="outline" size="sm" onClick={onNext} disabled={!hasNext}>
          {t("common.next")}
          <ChevronRight className="size-4" aria-hidden />
        </Button>
      </div>
    </div>
  );
}
