import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

export function PeopleFooter({
  shown,
  total,
  hasMore,
  onLoadMore,
}: {
  shown: number;
  total: number;
  hasMore: boolean;
  onLoadMore: () => void;
}) {
  const { t } = useTranslation();
  if (shown === 0) return null;

  return (
    <footer className="flex flex-wrap items-center gap-3 bg-muted px-4 py-3">
      <span className="text-xs text-muted-foreground tabular-nums">
        {t("people.footer", { shown, total })}
      </span>
      {hasMore && (
        <Button variant="outline" size="sm" onClick={onLoadMore}>
          {t("common.loadMore")}
        </Button>
      )}
      <span className="ml-auto max-w-xl text-xs text-text-disabled">
        {t("people.noRoleRule")}
      </span>
    </footer>
  );
}
