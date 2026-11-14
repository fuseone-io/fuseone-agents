import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useScopeLists } from "@/features/scope/use-scope-lists";

/**
 * Whether the companies and areas could be read. A value already chosen keeps
 * showing either way: its select lists it whatever the lists say. `message`
 * says what that means where the selects are.
 */
export function ScopeListsNotice({ message }: { message: string }) {
  const { t } = useTranslation();
  const { isLoading, error, refetch } = useScopeLists();
  if (isLoading) return <Skeleton className="h-4 w-48" />;
  if (!error) return null;
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
    >
      <span>{message}</span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7"
        onClick={refetch}
      >
        <RefreshCw className="size-3.5" aria-hidden />
        {t("common.retry")}
      </Button>
    </div>
  );
}
