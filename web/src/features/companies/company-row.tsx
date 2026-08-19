import { ChevronDown, ChevronUp, Pencil, RotateCcw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { Company } from "@/features/companies/api";
import { cn } from "@/lib/utils";

export function CompanyRow({
  company,
  open,
  onOpenChange,
  onEdit,
  onToggleArchived,
}: {
  company: Company;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onEdit: () => void;
  onToggleArchived: () => void;
}) {
  const { t } = useTranslation();
  const panelId = `company-detail-${cssId(company.id)}`;

  return (
    <div className="min-w-0">
      <Button
        type="button"
        variant="ghost"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => onOpenChange(!open)}
        className="grid h-auto w-full grid-cols-1 justify-stretch gap-3 rounded-none px-4 py-3 text-left font-normal whitespace-normal transition-colors hover:bg-muted/60 lg:grid-cols-[minmax(0,1.3fr)_112px_116px_36px] lg:items-center"
      >
        <CompanyName company={company} />
        <span className="text-xs text-muted-foreground tabular-nums">
          {t("companies.areaCount", { count: company.areas })}
        </span>
        <CompanyStatus archived={company.archived} />
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
            {t("companies.edit")}
          </Button>
          <Button
            type="button"
            variant={company.archived ? "outline" : "ghost"}
            size="sm"
            onClick={onToggleArchived}
          >
            {company.archived && <RotateCcw className="size-3.5" aria-hidden />}
            {company.archived
              ? t("companies.restore")
              : t("companies.withdraw")}
          </Button>
        </div>
      )}
    </div>
  );
}

function CompanyName({ company }: { company: Company }) {
  return (
    <div className="min-w-0">
      <p className="truncate text-sm font-medium">{company.label}</p>
      <Mono dim className="block truncate text-2xs">
        {company.id}
      </Mono>
    </div>
  );
}

function CompanyStatus({ archived }: { archived: boolean }) {
  const { t } = useTranslation();
  return (
    <Badge
      variant={archived ? "outline" : "secondary"}
      className={cn("w-fit", !archived && "text-success")}
    >
      {archived ? t("companies.withdrawn") : t("companies.active")}
    </Badge>
  );
}

function cssId(value: string) {
  return value.replace(/[^a-zA-Z0-9_-]/g, "-");
}
