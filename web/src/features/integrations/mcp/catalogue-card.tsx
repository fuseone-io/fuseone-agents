import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { stateOf } from "@/features/integrations/connection-state";
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
        <Button size="sm" variant={entry.configured ? "outline" : "default"} onClick={onOpen}>
          {entry.configured ? t("mcp.open") : t("mcp.connectIt")}
        </Button>
      </div>

      {/* The same four states the other card reads, from the same function.
          A configured server drawn as running would paint a switched-off one
          green and a refusing one greener. */}
      <div className="flex flex-wrap items-center gap-2 text-xs">
        {entry.configured ? (
          <>
            <Badge
              variant="outline"
              className={cn(
                "rounded-pill border-transparent text-2xs font-normal",
                stateOf(entry.enabled, entry.health).pill,
              )}
            >
              {t(stateOf(entry.enabled, entry.health).label)}
            </Badge>
            {entry.tools !== null && (
              <span className="text-muted-foreground">
                {t("mcp.offering", { count: entry.tools })}
              </span>
            )}
          </>
        ) : (
          <span className="text-muted-foreground">{t("mcp.notConnected")}</span>
        )}
      </div>

      {entry.description && (
        <p className="line-clamp-2 text-xs text-muted-foreground">
          {entry.description}
        </p>
      )}
    </article>
  );
}
