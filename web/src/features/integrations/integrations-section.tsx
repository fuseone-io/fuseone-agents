import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";

/** One kind of connection, with the control that adds another beside it —
 *  where the reader already is, rather than back up in the header. */
export function IntegrationsSection({
  title,
  onAdd,
  empty,
  children,
}: {
  title: string;
  onAdd: () => void;
  empty: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-2.5">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-medium">{title}</h2>
        <Button
          size="sm"
          variant="outline"
          className="ml-auto h-7"
          onClick={onAdd}
        >
          <Plus className="size-4" aria-hidden />
          {t("common.new")}
        </Button>
      </div>

      {empty || (
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(280px,1fr))]">
          {children}
        </div>
      )}
    </section>
  );
}
