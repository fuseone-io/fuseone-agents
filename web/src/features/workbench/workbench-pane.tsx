import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * One of the workbench's two panes: a card as tall as the screen allows,
 * whose header and footer stay put while its body scrolls. The page itself
 * does not scroll on a wide screen: the conversation and the draft each
 * scroll on their own, as NT-018's mockup draws them.
 */
export function WorkbenchPane({
  header,
  footer,
  children,
  label,
}: {
  header: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
  /** The pane's accessible name. */
  label: string;
}) {
  return (
    <section
      aria-label={label}
      className={cn("flex h-full min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border bg-card shadow-sm")}
    >
      <div className="shrink-0 border-b border-border">{header}</div>
      <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
      {footer && <div className="shrink-0 border-t border-border">{footer}</div>}
    </section>
  );
}
