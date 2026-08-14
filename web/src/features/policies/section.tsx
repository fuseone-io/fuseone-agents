import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

/**
 * A form section: a titled card with an optional clause saying what it decides.
 *
 * The icon is the same 24px tile every screen leads with, so a section reads as
 * a thing rather than as a paragraph break — and an author scrolling a long
 * form finds the part they want by its shape.
 */
export function Section({
  icon: Icon,
  title,
  hint,
  action,
  flush,
  children,
}: {
  icon?: LucideIcon;
  title: string;
  hint?: string;
  /** The one thing this section can do, beside its title. */
  action?: ReactNode;
  /** For a section whose content draws its own edges to the card's border. */
  flush?: boolean;
  children: ReactNode;
}) {
  return (
    <section
      className={cn(
        "flex flex-col rounded-xl border border-border bg-card shadow-sm",
        flush ? "overflow-hidden" : "gap-3 p-4",
      )}
    >
      <div
        className={cn(
          "flex items-center gap-2.5",
          flush && "border-b border-border px-4 py-3",
        )}
      >
        {Icon && (
          <span className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <Icon className="size-3.5" aria-hidden />
          </span>
        )}
        <div className="min-w-0">
          <h2 className="text-sm font-medium">{title}</h2>
          {hint && !flush && (
            <p className="text-xs text-muted-foreground">{hint}</p>
          )}
        </div>
        {/* Beside the title when the header is one line, so the caption and the
            action do not stack into a second row of chrome. */}
        {flush && hint && (
          <span className="min-w-0 truncate text-xs text-muted-foreground">
            {hint}
          </span>
        )}
        {action && <div className="ml-auto shrink-0">{action}</div>}
      </div>
      {children}
    </section>
  );
}

/** A field with its label above. A placeholder is not a label. */
export function Labelled({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label
        htmlFor={htmlFor}
        className="text-2xs uppercase tracking-label text-muted-foreground"
      >
        {label}
      </Label>
      {children}
    </div>
  );
}
