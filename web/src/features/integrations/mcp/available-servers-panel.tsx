import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Plug, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { CatalogueCard } from "@/features/integrations/mcp/catalogue-card";
import { CatalogueRail } from "@/features/integrations/mcp/catalogue-rail";
import {
  availableEntries,
  listing,
  matching,
  shelves,
} from "@/features/integrations/mcp/catalogue";

/**
 * Recipes not connected yet.
 *
 * Connected servers have their own tab now, because they are operational
 * objects with health and a surface. This panel keeps the catalogue focused on
 * the next act: choose a recipe, fill the connection form, and still decide
 * nothing about what any tool may do.
 */
export function AvailableServersPanel({
  servers,
  recipes,
  isLoading,
  error,
  onRetry,
}: {
  servers: MCPServer[];
  recipes: ServerRecipe[];
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [shelf, setShelf] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  const entries = useMemo(
    () => availableEntries(listing(servers, recipes)),
    [servers, recipes],
  );
  const shown = matching(
    shelf === null ? entries : entries.filter((one) => one.category === shelf),
    query,
  );

  if (isLoading) return <LoadingRows rows={4} />;
  if (error) return <ErrorState error={error} onRetry={onRetry} />;

  return (
    <div className="flex flex-col gap-6 sm:flex-row">
      <CatalogueRail counts={shelves(entries)} chosen={shelf} onChoose={setShelf} />

      <div className="min-w-0 flex-1 space-y-3">
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("mcp.searchServers")}
            aria-label={t("mcp.searchServers")}
          />
          <Button
            variant="outline"
            className="shrink-0"
            onClick={() => void navigate("/integrations/mcp/new")}
          >
            <Plus className="size-4" aria-hidden />
            {t("integrations.newServer")}
          </Button>
        </div>

        {shown.length === 0 ? (
          <EmptyState
            icon={<Plug className="size-6" />}
            title={t("mcp.nothingAvailable")}
            hint={t("mcp.nothingAvailableHint")}
          />
        ) : (
          <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,18rem),1fr))] gap-3">
            {shown.map((entry) => (
              <CatalogueCard
                key={entry.name}
                entry={entry}
                onOpen={() =>
                  void navigate(`/integrations/mcp/new?recipe=${entry.name}`)
                }
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
