import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * One part of the draft's summary: its title, its content read-only, and the
 * button that opens its manual controls, named for the part so a screen
 * reader can tell the buttons apart.
 */
export function SummarySection({
  title,
  onEdit,
  children,
}: {
  title: string;
  /** Absent for a part with no manual controls. */
  onEdit?: () => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-xs font-medium uppercase tracking-label text-muted-foreground">{title}</h3>
        {onEdit && (
          <Button variant="ghost" size="sm" onClick={onEdit} aria-label={t("workbench.editPart", { part: title.toLowerCase() })}>
            <Pencil className="size-3.5" aria-hidden />
            <span aria-hidden>{t("workbench.edit")}</span>
          </Button>
        )}
      </div>
      {children}
    </section>
  );
}

/** One fact of a part: what it is, and its value. */
export function SummaryFact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[8rem_minmax(0,1fr)] items-baseline gap-3 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 wrap-anywhere">{children}</dd>
    </div>
  );
}
