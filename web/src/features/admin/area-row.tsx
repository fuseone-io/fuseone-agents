import { ChevronDown, ChevronUp, Pencil } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { RemoveButton } from "@/components/shared/remove-button";
import type { RegisteredScope } from "@/features/scope/api";

export function AreaRow({
  area,
  open,
  onOpenChange,
  onEdit,
  onRemove,
}: {
  area: RegisteredScope;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const shown = area.label || area.area;
  const panelId = `area-detail-${cssId(area.company)}-${cssId(area.area)}`;

  return (
    <div className="min-w-0">
      <Button
        type="button"
        variant="ghost"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => onOpenChange(!open)}
        className="grid h-auto w-full grid-cols-1 justify-stretch gap-3 rounded-none px-4 py-3 text-left font-normal whitespace-normal transition-colors hover:bg-muted/60 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_112px_36px] lg:items-center"
      >
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{shown}</p>
          <Mono dim className="block truncate text-2xs">
            {area.area}
          </Mono>
        </div>
        <Mono dim className="block truncate text-xs">
          {area.company}
        </Mono>
        <span className="text-xs text-muted-foreground">
          {t("admin.expandForActions")}
        </span>
        <span className="hidden size-7 place-items-center rounded-md text-muted-foreground lg:grid">
          {open ? (
            <ChevronUp className="size-4" aria-hidden />
          ) : (
            <ChevronDown className="size-4" aria-hidden />
          )}
        </span>
      </Button>

      {open && (
        <div
          id={panelId}
          className="flex flex-wrap items-center gap-2 border-t bg-muted/35 px-4 py-3"
        >
          <Button type="button" variant="outline" size="sm" onClick={onEdit}>
            <Pencil className="size-3.5" aria-hidden />
            {t("admin.editArea")}
          </Button>
          <RemoveButton
            title={t("admin.removeArea", { area: shown })}
            description={t("admin.withdrawArea")}
            onConfirm={onRemove}
          />
        </div>
      )}
    </div>
  );
}

function cssId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]/g, "-");
}
