import type { ReactNode } from "react";

/** Title, one line of context, and at most one primary action. */
export function PageHeader({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex shrink-0 items-center gap-3">
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-2xl font-medium tracking-display">{title}</h1>
        {description && (
          <p className="truncate text-sm text-muted-foreground">{description}</p>
        )}
      </div>
      {children && <div className="flex shrink-0 items-center gap-2">{children}</div>}
    </div>
  );
}
