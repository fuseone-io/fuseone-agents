import { Skeleton } from "@/components/ui/skeleton";
import { LoadingRows } from "@/components/shared/states";

/**
 * What a screen looks like while its code is still on the wire.
 *
 * Shaped like every page: the icon tile and title of a PageHeader, then rows.
 * The shell around it is already drawn, so the reader sees the console settle
 * into place rather than a blank area or a spinner.
 */
export function PageFallback() {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <Skeleton className="size-[34px] rounded-md" />
        <div className="flex flex-col gap-2">
          <Skeleton className="h-6 w-48" />
          <Skeleton className="h-4 w-72" />
        </div>
      </div>
      <LoadingRows />
    </div>
  );
}
