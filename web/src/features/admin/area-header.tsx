import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

export function AreaHeader({ onAdd }: { onAdd: () => void }) {
  const { t } = useTranslation();
  return (
    <header className="flex flex-wrap items-start gap-3 border-b px-4 py-3.5">
      <div className="min-w-0 flex-1">
        <h2 className="text-base font-medium">{t("admin.areas")}</h2>
        <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
          {t("admin.areasSubtitle")}
        </p>
      </div>
      <Button variant="outline" size="sm" onClick={onAdd}>
        <Plus className="size-4" aria-hidden />
        {t("admin.newArea")}
      </Button>
    </header>
  );
}
