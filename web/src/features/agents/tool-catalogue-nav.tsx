import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { CatalogueNav } from "@/features/agents/tool-filtering";

/**
 * The catalogue's own navigation, down the left of the card.
 *
 * It exists because the catalogue belongs to the organisation and the choice
 * belongs to this agent, and those are two different things sharing one card.
 * The counts are what turn a list into a catalogue: "how much of the ERP can
 * it touch" is answered before anything is clicked.
 */
export function ToolCatalogueNav({
  entries,
  chosen,
  onChoose,
}: {
  entries: CatalogueNav[];
  chosen: string;
  onChoose: (server: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-2.5 border-r border-border p-3">
      <p className="px-1.5 text-2xs font-medium uppercase tracking-label text-muted-foreground">
        {t("agents.catalogue")}
      </p>

      <div className="flex flex-col gap-0.5">
        {entries.map((entry) => (
          <Button
            key={entry.server || "all"}
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => onChoose(entry.server)}
            aria-pressed={entry.server === chosen}
            className={cn(
              "h-8 justify-start gap-2 px-1.5 text-xs font-normal text-muted-foreground",
              entry.server === chosen && "bg-muted font-medium text-foreground",
            )}
          >
            <span className="min-w-0 flex-1 truncate text-left">
              {entry.label}
            </span>
            <span className="shrink-0 font-mono text-2xs tabular-nums opacity-70">
              {entry.count}
            </span>
          </Button>
        ))}
      </div>

      {/* Under the entries rather than pinned to the bottom. Pinned, it was
          measured before it had a width to wrap in and overflowed the card's
          edge; and with a real catalogue the list is long enough that it lands
          near the bottom anyway. */}
      <p className="rounded-md bg-muted/50 p-2 text-2xs text-muted-foreground">
        {t("agents.catalogueIsTheOrgs")}
      </p>
    </div>
  );
}
