import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * The console's dense panel: a hairline border does the containing, a soft
 * shadow does the lifting.
 *
 * Distinct from `Card`, which carries shadcn's roomier rhythm. Console rows
 * are compact; a panel that breathes like a marketing card halves how much of
 * a run an operator can see at once.
 */
export function Panel({
  title,
  action,
  children,
  flush,
  className,
}: {
  title?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  /** Content that draws its own edges — a table, a list — sits flush. */
  flush?: boolean;
  className?: string;
}) {
  return (
    <section
      className={cn(
        "overflow-hidden rounded-xl border bg-card shadow-sm",
        className,
      )}
    >
      {title && (
        <header className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-3">
          <h2 className="min-w-0 flex-1 text-base font-medium">{title}</h2>
          {action && (
            <div className="flex min-w-0 items-center gap-2">{action}</div>
          )}
        </header>
      )}
      <div className={cn(!flush && "p-4")}>{children}</div>
    </section>
  );
}
