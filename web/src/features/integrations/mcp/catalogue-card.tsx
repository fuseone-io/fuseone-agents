import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { Listing } from "@/features/integrations/mcp/catalogue";
import { cn } from "@/lib/utils";

/**
 * One recipe on the available shelf.
 *
 * The publisher is here and this platform is never it. A card that showed only
 * a name and a logo would read as a catalogue of things we supply.
 */
export function CatalogueCard({
  entry,
  onOpen,
  selected,
}: {
  entry: Listing;
  onOpen: () => void;
  selected?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <article
      className={cn(
        "flex flex-col gap-2 rounded-xl border p-3 shadow-sm",
        selected && "border-primary ring-1 ring-primary/30",
      )}
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{entry.title}</p>
          <p className="truncate text-xs text-muted-foreground">
            {entry.publisher
              ? t("mcp.publishedBy", { publisher: entry.publisher })
              : t("mcp.publisherUnknown")}
          </p>
        </div>
        <Button
          size="sm"
          variant="default"
          onClick={onOpen}
          className="self-start"
          aria-label={t("mcp.connectNamed", { name: entry.title })}
        >
          {t("mcp.connectIt")}
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="text-muted-foreground">{t("mcp.notConnected")}</span>
      </div>

      {entry.description && (
        <p className="line-clamp-2 text-xs text-muted-foreground">
          {entry.description}
        </p>
      )}
    </article>
  );
}
