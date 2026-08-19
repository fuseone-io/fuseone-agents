import { Search } from "lucide-react";
import { useTranslation } from "react-i18next";

export function AreaToolbar({
  search,
  onSearch,
  total,
  companies,
}: {
  search: string;
  onSearch: (value: string) => void;
  total: number;
  companies: number;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2 border-b bg-muted px-4 py-3">
      <label className="flex h-8 min-w-[200px] max-w-[280px] flex-1 items-center gap-2 rounded-md border border-input bg-card px-2.5 focus-within:shadow-[var(--elevation-focus)]">
        <Search className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="sr-only">{t("admin.searchAreas")}</span>
        <input
          type="search"
          value={search}
          onChange={(event) => onSearch(event.target.value)}
          placeholder={t("admin.searchAreas")}
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
      </label>

      <span className="ml-auto text-xs text-muted-foreground">
        {t("admin.areasSummary", { count: total, companies })}
      </span>
    </div>
  );
}
