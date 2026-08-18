import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { BookOpen } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { useManual } from "@/features/manual/api";

/** What the manual has, in the order it was written to be read. */
export function ManualIndexPage() {
  const { t } = useTranslation();
  const manual = useManual();

  return (
    <>
      <PageHeader icon={BookOpen} title={t("manual.title")} description={t("manual.subtitle")} />
      {manual.isLoading && <LoadingRows rows={4} />}
      {manual.error && <ErrorState error={manual.error} onRetry={() => void manual.refetch()} />}
      {manual.data?.pages.length === 0 && (
        <EmptyState title={t("manual.emptyTitle")} hint={t("manual.emptyHelp")} />
      )}
      <div className="grid gap-3 md:grid-cols-2">
        {manual.data?.pages.map((page) => (
          <Link
            key={page.slug}
            to={`/manual/${page.slug}`}
            className="min-w-0 rounded-xl border border-border bg-card p-4 shadow-sm transition-colors hover:border-primary"
          >
            <h2 className="font-medium break-words">{page.title}</h2>
            <p className="mt-1 text-sm text-muted-foreground break-words">
              {page.summary}
            </p>
          </Link>
        ))}
      </div>
    </>
  );
}
