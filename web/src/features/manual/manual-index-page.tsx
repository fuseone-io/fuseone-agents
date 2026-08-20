import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { BookOpen } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { useManual } from "@/features/manual/api";
import { ManualPageCard } from "@/features/manual/manual-page-card";
import { groupManualPages, searchManual } from "@/features/manual/manual-search";
import { ManualSearchBox } from "@/features/manual/manual-search-box";
import { ManualSectionNav } from "@/features/manual/manual-section-nav";
import type { ManualEntry } from "@/features/manual/api";

const emptyManualPages: ManualEntry[] = [];

/** What the manual has, in the order it was written to be read. */
export function ManualIndexPage() {
  const { t } = useTranslation();
  const manual = useManual();
  const [query, setQuery] = useState("");
  const pages = manual.data?.pages ?? emptyManualPages;
  const filtered = useMemo(() => searchManual(pages, query), [pages, query]);
  const groups = useMemo(() => groupManualPages(filtered), [filtered]);

  return (
    <>
      <PageHeader icon={BookOpen} title={t("manual.title")} description={t("manual.subtitle")} />
      {manual.isLoading && <LoadingRows rows={4} />}
      {manual.error && <ErrorState error={manual.error} onRetry={() => void manual.refetch()} />}
      {manual.data?.pages.length === 0 && (
        <EmptyState title={t("manual.emptyTitle")} hint={t("manual.emptyHelp")} />
      )}
      {manual.data && (
        <div className="grid min-w-0 gap-6 lg:grid-cols-[220px_minmax(0,1fr)]">
          <ManualSectionNav pages={pages} />
          <div className="min-w-0 space-y-5">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <ManualSearchBox value={query} onChange={setQuery} />
              <p className="text-sm text-muted-foreground">
                {t("manual.resultCount", { count: filtered.length })}
              </p>
            </div>
            {groups.length === 0 ? (
              <EmptyState title={t("manual.noMatches")} hint={t("manual.noMatchesHelp")} />
            ) : (
              groups.map((group) => (
                <section key={group.section} id={group.section} className="scroll-mt-20">
                  <div className="mb-3">
                    <h2 className="font-medium">{t(`manual.sections.${group.section}`)}</h2>
                  </div>
                  <div className="grid min-w-0 gap-3 xl:grid-cols-2">
                    {group.pages.map((page) => (
                      <ManualPageCard key={page.slug} page={page} />
                    ))}
                  </div>
                </section>
              ))
            )}
          </div>
        </div>
      )}
    </>
  );
}
