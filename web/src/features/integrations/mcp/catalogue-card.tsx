import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { StateDot } from "@/components/shared/state-dot";
import type { Listing } from "@/features/integrations/mcp/catalogue";

/**
 * One entry on the shelf, saying what it is and whether we run it.
 *
 * Connectedness is a dot and a word, never colour alone, and it is the first
 * thing on the card: somebody scanning this page is asking what the
 * installation reaches, and everything else on the card is context for that
 * answer.
 *
 * The publisher is here and this platform is never it. A card that showed only
 * a name and a logo would read as a catalogue of things we supply.
 */
export function CatalogueCard({
  entry,
  onOpen,
}: {
  entry: Listing;
  onOpen: () => void;
}) {
  const { t } = useTranslation();

  return (
    <article className="flex flex-col gap-2 rounded-xl border p-3 shadow-sm">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{entry.title}</p>
          <p className="truncate text-xs text-muted-foreground">
            {entry.publisher
              ? t("mcp.publishedBy", { publisher: entry.publisher })
              : t("mcp.publisherUnknown")}
          </p>
        </div>
        <Button size="sm" variant={entry.connected ? "outline" : "default"} onClick={onOpen}>
          {entry.connected ? t("mcp.open") : t("mcp.connectIt")}
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-2 text-xs">
        <StateDot state={entry.connected ? "running" : "draft"} />
        <span className={entry.connected ? "" : "text-muted-foreground"}>
          {entry.connected
            ? t("mcp.connectedWith", { count: entry.tools ?? 0 })
            : t("mcp.notConnected")}
        </span>
      </div>

      {entry.description && (
        <p className="line-clamp-2 text-xs text-muted-foreground">
          {entry.description}
        </p>
      )}
    </article>
  );
}
