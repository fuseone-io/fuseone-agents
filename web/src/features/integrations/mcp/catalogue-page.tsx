import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { Plug } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { Input } from "@/components/ui/input";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { useIntegrations } from "@/features/integrations/api";
import { useRecipes } from "@/features/integrations/mcp/api";
import { CatalogueRail } from "@/features/integrations/mcp/catalogue-rail";
import { CatalogueCard } from "@/features/integrations/mcp/catalogue-card";
import { listing, matching, shelves } from "@/features/integrations/mcp/catalogue";

/**
 * What this installation can reach, and what it knows how to reach.
 *
 * One list rather than two. "What tools do we have" is answered by the servers
 * we run and the ones we know how to connect, and splitting them makes
 * somebody check both lists to discover that a server they already run is
 * already running.
 *
 * Nothing on this page grants anything. Connecting a server discovers its
 * tools and every one arrives unclassified and refused; bringing a tool in is
 * a separate act, and saying what it does is a third. The page is ordered that
 * way because the decisions are.
 */
export function CataloguePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const integrations = useIntegrations();
  const recipes = useRecipes();
  const [shelf, setShelf] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  const entries = useMemo(
    () => listing(integrations.data?.mcpServers ?? [], recipes.data?.items ?? []),
    [integrations.data, recipes.data],
  );

  const shown = matching(
    shelf === null ? entries : entries.filter((one) => one.category === shelf),
    query,
  );

  if (integrations.isLoading || recipes.isLoading) return <LoadingRows rows={4} />;
  /*
    Either read failing is an error, and neither is an empty list.

    A recipes call that failed used to render as "we know about nothing",
    which is a claim rather than a failure — and the one thing a catalogue must
    never say when it simply could not look.
  */
  const failed = integrations.error ?? recipes.error;
  if (failed) {
    return (
      <ErrorState
        error={failed}
        onRetry={() => {
          void integrations.refetch();
          void recipes.refetch();
        }}
      />
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Plug}
        title={t("mcp.catalogue")}
        description={t("mcp.catalogueDescription")}
      />

      <div className="flex flex-col gap-6 sm:flex-row">
        <CatalogueRail counts={shelves(entries)} chosen={shelf} onChoose={setShelf} />

        <div className="min-w-0 flex-1 space-y-3">
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("mcp.searchServers")}
            aria-label={t("mcp.searchServers")}
          />

          {shown.length === 0 ? (
            <EmptyState
              icon={<Plug className="size-6" />}
              title={t("mcp.nothingHere")}
              hint={t("mcp.nothingHereHint")}
            />
          ) : (
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {shown.map((entry) => (
                <CatalogueCard
                  key={entry.name}
                  entry={entry}
                  onOpen={() =>
                    void navigate(
                      entry.configured
                        ? `/integrations/mcp/${entry.name}`
                        : `/integrations/mcp/new?recipe=${entry.name}`,
                    )
                  }
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
