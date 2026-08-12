import type { ReactNode } from "react";
import { Label } from "@/components/ui/label";

/** A form section: a titled card with an optional clause saying what it decides. */
export function Section({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <div>
        <h2 className="text-sm font-medium">{title}</h2>
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
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
      <Label htmlFor={htmlFor} className="text-2xs uppercase tracking-label text-muted-foreground">
        {label}
      </Label>
      {children}
    </div>
  );
}
