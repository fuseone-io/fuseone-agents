import { useTranslation } from "react-i18next";
import { Skeleton } from "@/components/ui/skeleton";

/**
 * The console's outline while the console itself is loading: a sidebar and a
 * flush content area, the same geometry AppShell is about to draw.
 *
 * It shows once per session, just after sign-in — the sign-in screen does not
 * carry the console, so the console is fetched when it is first needed.
 */
export function ShellFallback() {
  const { t } = useTranslation();

  return (
    <div className="flex h-svh bg-background" aria-busy="true">
      <span className="sr-only">{t("common.loading")}</span>
      <Skeleton className="hidden h-full w-[248px] rounded-none md:block" />
      <div className="flex flex-1 flex-col gap-6 p-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    </div>
  );
}
