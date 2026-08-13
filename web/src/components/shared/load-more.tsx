import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

interface LoadMoreProps {
  loaded: number;
  total?: number;
  hasMore: boolean;
  isLoading: boolean;
  onLoad: () => void;
}

/**
 * The foot of a list that continues past what is on screen.
 *
 * It always says how many rows are loaded, because the alternative is the
 * state this replaces: a screen reporting a total taken over the whole set
 * beside a list that stopped at the limit, with no way to reach the rest and
 * nothing saying so.
 */
export function LoadMore({
  loaded,
  total,
  hasMore,
  isLoading,
  onLoad,
}: LoadMoreProps) {
  const { t } = useTranslation();
  if (loaded === 0) return null;

  return (
    <div className="flex items-center justify-between gap-4 pt-2">
      <p className="text-xs text-muted-foreground tabular-nums">
        {total !== undefined
          ? t("common.showingOf", { loaded, total })
          : t("common.showing", { loaded })}
      </p>
      {hasMore && (
        <Button
          variant="outline"
          size="sm"
          onClick={onLoad}
          disabled={isLoading}
        >
          {isLoading ? t("common.loadingMore") : t("common.loadMore")}
        </Button>
      )}
    </div>
  );
}
