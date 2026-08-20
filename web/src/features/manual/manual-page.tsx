import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, BookOpen } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { ManualBody } from "@/features/manual/manual-body";
import { ManualPageOutline } from "@/features/manual/manual-page-outline";
import { useManualPage } from "@/features/manual/api";

/** One page, read. */
export function ManualReadPage() {
  const { t } = useTranslation();
  const { slug = "" } = useParams();
  const page = useManualPage(slug);

  return (
    <>
      <PageHeader
        icon={BookOpen}
        title={page.data?.title ?? t("manual.title")}
        description={page.data?.summary}
      >
        <Button asChild variant="outline" size="sm">
          <Link to="/manual">
            <ArrowLeft className="size-4" aria-hidden />
            {t("manual.back")}
          </Link>
        </Button>
      </PageHeader>
      {page.isLoading && <LoadingRows rows={6} />}
      {page.error && <ErrorState error={page.error} onRetry={() => void page.refetch()} />}
      {page.data && (
        <div className="grid min-w-0 gap-8 lg:grid-cols-[minmax(0,1fr)_260px]">
          <ManualBody body={page.data.body} headings={page.data.headings} />
          <ManualPageOutline page={page.data} />
        </div>
      )}
    </>
  );
}
