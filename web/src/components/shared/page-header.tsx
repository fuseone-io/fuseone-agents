import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { PageActions } from "@/components/layout/page-actions";

/**
 * Title, one line of context, and at most one primary action.
 *
 * The icon tile is the same device as the 24px tiles on section headers, one
 * step larger — so a screen and the panels inside it read as the same system
 * rather than as a heading followed by unrelated boxes.
 *
 * In edit modes it precedes the record's identifier and never replaces it: an
 * icon says which kind of thing this is, and only the name says which one.
 */
export function PageHeader({
  icon: Icon,
  title,
  description,
  children,
}: {
  icon?: LucideIcon;
  title: string;
  description?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex shrink-0 items-center gap-3">
      {Icon && (
        <span
          aria-hidden
          className="flex size-[34px] shrink-0 items-center justify-center rounded-md border border-border bg-muted text-muted-foreground"
        >
          <Icon className="size-[17px]" />
        </span>
      )}

      <div className="min-w-0 flex-1">
        <h1 className="truncate text-2xl font-medium tracking-display">{title}</h1>
        {description && (
          <p className="truncate text-sm text-muted-foreground">{description}</p>
        )}
      </div>

      {/* Rendered in the header, beside the theme toggle. A screen's one
          primary action belongs to the chrome: it stays reachable when the
          page is scrolled, and it stops each page inventing its own place. */}
      {children && <PageActions>{children}</PageActions>}
    </div>
  );
}
