import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { BookOpen, Plug, Plus, Server, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Form } from "@/components/ui/form";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { CatalogueCard } from "@/features/integrations/mcp/catalogue-card";
import { CatalogueIcon } from "@/features/integrations/mcp/catalogue-icons";
import {
  ConfigRequirementBadges,
  RecipeStatusBadge,
} from "@/features/integrations/mcp/recipe-badges";
import { CatalogueRail } from "@/features/integrations/mcp/catalogue-rail";
import { ServerFormBody } from "@/features/integrations/server-form-body";
import { useServerForm } from "@/features/integrations/mcp/use-server-form";
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
  const [panel, setPanel] = useState<string | null | undefined>(undefined);

  const entries = useMemo(
    () => availableEntries(listing(servers, recipes)),
    [servers, recipes],
  );
  const selectedRecipe = panel
    ? recipes.find((recipe) => recipe.server === panel) ?? null
    : null;
  const shown = matching(
    shelf === null ? entries : entries.filter((one) => one.category === shelf),
    query,
  );

  if (isLoading) return <LoadingRows rows={4} />;
  if (error) return <ErrorState error={error} onRetry={onRetry} />;

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_24rem]">
      <div className="flex min-w-0 flex-col gap-6 sm:flex-row">
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
              onClick={() => setPanel(null)}
            >
              <Plus className="size-4" aria-hidden />
              {t("mcp.connectByAddress")}
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
                  selected={panel === entry.name}
                  onOpen={() => setPanel(entry.name)}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      <ConnectServerPanel
        key={panel === undefined ? "closed" : (panel ?? "custom")}
        open={panel !== undefined}
        recipe={selectedRecipe}
        onClose={() => setPanel(undefined)}
        onConnected={(name) => void navigate(`/integrations/mcp/${name}`)}
      />
    </div>
  );
}

function ConnectServerPanel({
  open,
  recipe,
  onClose,
  onConnected,
}: {
  open: boolean;
  recipe: ServerRecipe | null;
  onClose: () => void;
  onConnected: (name: string) => void;
}) {
  const { t } = useTranslation();
  const initial = recipe
    ? {
        name: recipe.server,
        transport: recipe.transport ?? "stdio",
        command: recipe.command ?? "",
        args: recipe.args ?? [],
        url: recipe.url ?? "",
        acceptsLocalExecution: false,
        enabled: true,
      }
    : null;
  const { form, submit, saving } = useServerForm(initial, onConnected);
  const title = recipe?.title ?? t("mcp.customServer");

  if (!open) {
    return (
      <aside className="rounded-xl border bg-card p-4 shadow-sm xl:sticky xl:top-4 xl:self-start">
        <EmptyState
          icon={<Server className="size-6" />}
          title={t("mcp.chooseServer")}
          hint={t("mcp.chooseServerHint")}
        />
      </aside>
    );
  }

  return (
    <aside className="overflow-hidden rounded-xl border bg-card shadow-sm xl:sticky xl:top-4 xl:self-start">
      <header className="flex items-start gap-3 border-b p-4">
        <CatalogueIcon
          entry={{
            name: recipe?.server ?? "custom",
            category: recipe?.category ?? "operations",
          }}
          className="size-9"
        />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-base font-medium">
            {t("mcp.connectionFor", { name: title })}
          </h2>
          <p className="truncate text-xs text-muted-foreground">
            {recipe
              ? t("mcp.publishedBy", { publisher: recipe.publisher })
              : t("mcp.customServerHint")}
          </p>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-8"
          onClick={onClose}
          aria-label={t("common.close")}
        >
          <X className="size-4" aria-hidden />
        </Button>
      </header>

      <div className="space-y-4 p-4">
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {recipe?.note ?? t("mcp.connectPanelHint")}
          </p>
          {recipe && (
            <div className="flex flex-wrap items-center gap-2 text-2xs text-muted-foreground">
              <RecipeStatusBadge status={recipe.status} />
              <ConfigRequirementBadges requirements={recipe.configRequirements} />
              <span className="rounded-md bg-muted px-2 py-1">
                {t(`mcp.docsFrom.${recipe.docsFrom}`)}
              </span>
              <span className="rounded-md bg-muted px-2 py-1">
                {t(`mcp.provenance.${recipe.provenance}`)}
              </span>
              {recipe.docs && (
                <a
                  href={recipe.docs}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="inline-flex items-center gap-1 underline underline-offset-2"
                >
                  <BookOpen className="size-3" aria-hidden />
                  {t("mcp.readTheDocs")}
                </a>
              )}
            </div>
          )}
          {recipe?.status === "archived" && (
            <p className="rounded-lg border border-danger/30 bg-danger-surface px-3 py-2 text-xs text-danger">
              {t("mcp.archivedRecipeWarning")}
            </p>
          )}
          {recipe?.configRequirements.includes("file") && (
            <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning">
              {t("mcp.configFileWarning")}
            </p>
          )}
        </div>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <ServerFormBody form={form} editing={false} hasSecret={false} />
            <div className="flex justify-end gap-2 border-t pt-4">
              <Button type="button" variant="ghost" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saving}>
                {t("integrations.connect")}
              </Button>
            </div>
          </form>
        </Form>
      </div>
    </aside>
  );
}
