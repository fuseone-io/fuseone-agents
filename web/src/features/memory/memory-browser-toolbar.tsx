import { Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { MemoryFilters } from "@/features/memory/api";
import type { MemoryView } from "@/features/memory/memory-view";

export function MemoryBrowserToolbar({
  filters,
  view,
  onFilters,
  onView,
}: {
  filters: MemoryFilters;
  view: MemoryView;
  onFilters: (filters: MemoryFilters) => void;
  onView: (view: MemoryView) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-start gap-3 border-b px-4 py-3">
      <div className="relative min-w-56 flex-1">
        <Search
          className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden
        />
        <Input
          type="search"
          aria-label={t("memory.search")}
          placeholder={t("memory.searchPlaceholder")}
          value={filters.search}
          onChange={(event) => onFilters({ ...filters, search: event.target.value })}
          className="h-8 pl-8"
        />
      </div>
      <Input
        aria-label={t("memory.agentFilter")}
        placeholder={t("memory.agentFilterPlaceholder")}
        value={filters.agentId}
        onChange={(event) => onFilters({ ...filters, agentId: event.target.value })}
        className="h-8 min-w-44 flex-1 font-mono sm:max-w-56"
      />
      <Tabs value={view} onValueChange={(next) => onView(next as MemoryView)}>
        <TabsList className="h-8" aria-label={t("memory.viewFilter")}>
          <TabsTrigger value="active">{t("memory.status.active")}</TabsTrigger>
          <TabsTrigger value="disabled">{t("memory.status.disabled")}</TabsTrigger>
          <TabsTrigger value="suggested">{t("memory.suggestions")}</TabsTrigger>
          <TabsTrigger value="all">{t("memory.status.all")}</TabsTrigger>
        </TabsList>
      </Tabs>
    </div>
  );
}
