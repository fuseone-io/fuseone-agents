import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useGrantScopes } from "@/features/admin/use-grant-scopes";

/**
 * Whether the companies and areas could be read. A grant already held keeps
 * showing either way: its row lists what it names whatever the lists say.
 */
export function GrantScopeNotice() {
  const { t } = useTranslation();
  const { isLoading, error, refetch } = useGrantScopes();
  if (isLoading) return <Skeleton className="h-4 w-48" />;
  if (!error) return null;
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
    >
      <span>{t("people.scopesFailed")}</span>
      <Button variant="outline" size="sm" className="h-7" onClick={refetch}>
        <RefreshCw className="size-3.5" aria-hidden />
        {t("common.retry")}
      </Button>
    </div>
  );
}
