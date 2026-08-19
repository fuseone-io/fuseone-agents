import { Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  PEOPLE_VIEWS,
  type PeopleView,
} from "@/features/admin/people-filters";
import { cn } from "@/lib/utils";

export function PeopleToolbar({
  search,
  onSearch,
  view,
  onView,
  noRole,
  local,
}: {
  search: string;
  onSearch: (value: string) => void;
  view: PeopleView;
  onView: (view: PeopleView) => void;
  noRole: number;
  local: number;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2 border-b bg-muted px-4 py-3">
      <label className="flex h-8 min-w-[200px] max-w-[280px] flex-1 items-center gap-2 rounded-md border border-input bg-card px-2.5 focus-within:shadow-[var(--elevation-focus)]">
        <Search
          className="size-3.5 shrink-0 text-muted-foreground"
          aria-hidden
        />
        <span className="sr-only">{t("people.search")}</span>
        <input
          type="search"
          value={search}
          onChange={(event) => onSearch(event.target.value)}
          placeholder={t("people.search")}
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
      </label>

      <div
        className="inline-flex h-8 shrink-0 items-center gap-0.5 rounded-lg border bg-background p-0.5"
        aria-label={t("people.viewFilter")}
      >
        {PEOPLE_VIEWS.map((candidate) => (
          <Button
            key={candidate}
            type="button"
            variant="ghost"
            size="xs"
            aria-pressed={view === candidate}
            onClick={() => onView(candidate)}
            className={cn(
              "h-6 rounded-md px-2.5 text-xs transition-colors",
              view === candidate
                ? "border border-border bg-card text-foreground shadow-sm hover:bg-card hover:text-foreground"
                : "border border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {t(`people.views.${candidate}`)}
          </Button>
        ))}
      </div>

      <span className="ml-auto text-xs text-muted-foreground">
        {t("people.summary", { noRole, local })}
      </span>
    </div>
  );
}
