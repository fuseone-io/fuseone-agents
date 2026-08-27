import { Archive, Building2, CircleCheck, Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  COMPANY_VIEWS,
  type CompanyView,
} from "@/features/companies/company-filters";
import { cn } from "@/lib/utils";

const VIEW_ICONS = {
  all: Building2,
  active: CircleCheck,
  withdrawn: Archive,
} satisfies Record<CompanyView, typeof Search>;

export function CompanyToolbar({
  search,
  onSearch,
  view,
  onView,
  active,
  withdrawn,
}: {
  search: string;
  onSearch: (value: string) => void;
  view: CompanyView;
  onView: (view: CompanyView) => void;
  active: number;
  withdrawn: number;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2 border-b bg-muted px-4 py-3">
      <label className="flex h-8 min-w-[200px] max-w-[280px] flex-1 items-center gap-2 rounded-md border border-input bg-card px-2.5 focus-within:shadow-[var(--elevation-focus)]">
        <Search className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="sr-only">{t("companies.search")}</span>
        <input
          type="search"
          value={search}
          onChange={(event) => onSearch(event.target.value)}
          placeholder={t("companies.search")}
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
      </label>

      <div
        className="inline-flex h-8 shrink-0 items-center gap-0.5 rounded-lg border bg-background p-0.5"
        aria-label={t("companies.viewFilter")}
      >
        {COMPANY_VIEWS.map((candidate) => (
          <CompanyViewButton
            key={candidate}
            view={candidate}
            active={view === candidate}
            onClick={() => onView(candidate)}
          />
        ))}
      </div>

      <span className="ml-auto text-xs text-muted-foreground">
        {t("companies.summary", { active, withdrawn })}
      </span>
    </div>
  );
}

function CompanyViewButton({
  view,
  active,
  onClick,
}: {
  view: CompanyView;
  active: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const Icon = VIEW_ICONS[view];

  return (
    <Button
      type="button"
      variant="ghost"
      size="xs"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        "h-6 rounded-md px-2.5 text-xs transition-colors",
        active
          ? "border border-border bg-card text-foreground shadow-sm hover:bg-card hover:text-foreground"
          : "border border-transparent text-muted-foreground hover:text-foreground",
      )}
    >
      <Icon className="size-3.5" aria-hidden />
      {t(`companies.views.${view}`)}
    </Button>
  );
}
