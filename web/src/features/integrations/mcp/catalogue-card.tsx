import { useTranslation } from "react-i18next";
import type { Listing } from "@/features/integrations/mcp/catalogue";
import { CatalogueIcon } from "@/features/integrations/mcp/catalogue-icons";
import {
  AuthModeBadges,
  ConfigRequirementBadges,
  RecipeStatusBadge,
} from "@/features/integrations/mcp/recipe-badges";
import { stateOf } from "@/features/integrations/connection-state";
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
  const state = entry.configured
    ? stateOf(entry.enabled, entry.health)
    : null;

  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "flex min-h-[166px] flex-col gap-2 rounded-lg border bg-card p-3 text-left shadow-sm transition-colors hover:border-border-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        selected && "border-primary ring-1 ring-primary/30",
      )}
      aria-label={t(entry.configured ? "mcp.configureNamed" : "mcp.connectNamed", {
        name: entry.title,
      })}
      aria-pressed={selected}
    >
      <div className="flex items-center gap-3">
        <CatalogueIcon entry={entry} className="size-8 rounded-md" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{entry.title}</p>
          <p className="truncate text-xs text-muted-foreground">
            {entry.publisher
              ? t("mcp.publisherLine", { publisher: entry.publisher })
              : t("mcp.publisherUnknown")}
          </p>
        </div>
      </div>

      {entry.description && (
        <p className="line-clamp-2 min-h-9 text-xs text-muted-foreground">
          {entry.description}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-1.5 text-xs">
        <span className="font-mono text-2xs text-muted-foreground">
          {entry.tools === null
            ? t("mcp.toolsUnknown")
            : t("mcp.offering", { count: entry.tools })}
        </span>
        {entry.status && <RecipeStatusBadge status={entry.status} />}
        <ConfigRequirementBadges requirements={entry.configRequirements} />
        <AuthModeBadges modes={entry.authModes} />
      </div>

      <div className="mt-auto flex items-center gap-2 border-t border-border-subtle pt-2">
        <span className="inline-flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <span
            className={cn(
              "size-1.5 rounded-full",
              entry.configured
                ? state?.pill.includes("success")
                  ? "bg-success"
                  : state?.pill.includes("danger")
                    ? "bg-danger"
                    : state?.pill.includes("warning")
                      ? "bg-warning"
                      : "bg-border-strong"
                : "bg-border-strong",
            )}
            aria-hidden
          />
          <span className="truncate">
            {entry.configured && state
              ? t(state.label)
              : t("mcp.notConnected")}
          </span>
        </span>
        <span className="ml-auto rounded-md border px-2 py-1 text-2xs font-medium">
          {entry.configured ? t("mcp.configure") : t("mcp.connectIt")}
        </span>
      </div>
    </button>
  );
}
