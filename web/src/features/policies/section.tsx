import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

/**
 * A form section: a card with a header that says what it decides.
 *
 * The header is separated by a rule and led by the same 24px icon tile every
 * screen uses, so a section reads as a thing rather than as a paragraph break —
 * and an author scrolling a long form finds the part they want by its shape
 * rather than by reading every heading.
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
  /** For content that draws its own edges to the card's border. */
  flush?: boolean;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <div className="flex items-center gap-2.5 border-b border-border px-4 py-3">
        {Icon && (
          <span className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <Icon className="size-3.5" aria-hidden />
          </span>
        )}
        <h2 className="shrink-0 text-sm font-medium">{title}</h2>
        {/* Beside the title rather than under it: the header is one line, and
            a caption on a second row doubles the chrome above every field. */}
        {hint && (
          <p className="min-w-0 truncate text-xs text-muted-foreground">
            {hint}
          </p>
        )}
        {action && <div className="ml-auto shrink-0">{action}</div>}
      </div>

      <div className={cn(!flush && "flex flex-col gap-3 p-4")}>{children}</div>
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
