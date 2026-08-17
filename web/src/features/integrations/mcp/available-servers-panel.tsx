import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  BookOpen,
  KeyRound,
  LinkIcon,
  Plug,
  Plus,
  Server,
  ShieldCheck,
  X,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Form } from "@/components/ui/form";
import type { Tool } from "@/features/admin/api";
import { EffectBadge } from "@/features/admin/effect-badge";
import type { MCPServer } from "@/features/integrations/api";
import { stateOf } from "@/features/integrations/connection-state";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import { CatalogueCard } from "@/features/integrations/mcp/catalogue-card";
import { CatalogueIcon } from "@/features/integrations/mcp/catalogue-icons";
import {
  AuthModeBadges,
  ConfigRequirementBadges,
  RecipeStatusBadge,
} from "@/features/integrations/mcp/recipe-badges";
import { CatalogueRail } from "@/features/integrations/mcp/catalogue-rail";
import { ServerFormBody } from "@/features/integrations/server-form-body";
import { useServerForm } from "@/features/integrations/mcp/use-server-form";
import { remoteNameOf } from "@/features/integrations/mcp/tool-names";
import { useVisibleItems } from "@/hooks/use-visible-items";
import {
  CONNECTED_SHELF,
  listing,
  matching,
  shelves,
  type Listing,
} from "@/features/integrations/mcp/catalogue";
import { cn } from "@/lib/utils";

const ORIGINS = ["all", "published", "reference", "archived"] as const;
type OriginFilter = (typeof ORIGINS)[number];

/**
 * The MCP catalogue, in the handoff shape: shelves on the left, servers in the
 * middle, and the selected server's properties on the right.
 *
 * Recipes and configured servers are one list because "what can this
 * installation reach" has one answer. Connected is a filter over that list,
 * not a separate catalogue that can hide a server from the place somebody is
 * looking.
 */
export function AvailableServersPanel({
  servers,
  recipes,
  tools = [],
  isLoading,
  error,
  onRetry,
}: {
  servers: MCPServer[];
  recipes: ServerRecipe[];
  tools?: Tool[];
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [shelf, setShelf] = useState<string | null>(null);
  const [origin, setOrigin] = useState<OriginFilter>("all");
  const [query, setQuery] = useState("");
  const [panel, setPanel] = useState<string | null | undefined>(undefined);

  const entries = useMemo(() => listing(servers, recipes), [servers, recipes]);
  const connected = entries.filter((entry) => entry.configured).length;
  const serversByName = useMemo(
    () => new Map(servers.map((server) => [server.name, server])),
    [servers],
  );
  const toolsByServer = useMemo(() => groupTools(tools), [tools]);
  const toolSearch = useMemo(() => toolSearchTerms(tools), [tools]);
  const chosenEntry = panel
    ? entries.find((entry) => entry.name === panel) ?? null
    : null;
  const chosenServer = chosenEntry?.configured
    ? serversByName.get(chosenEntry.name) ?? null
    : null;
  const chosenRecipe = chosenEntry?.recipe ?? null;

  const filtered = useMemo(() => {
    const byShelf =
      shelf === CONNECTED_SHELF
        ? entries.filter((entry) => entry.configured)
        : shelf === null
          ? entries
          : entries.filter((entry) => entry.category === shelf);
    const byOrigin =
      origin === "all"
        ? byShelf
        : byShelf.filter((entry) => entry.status === origin);
    return matching(byOrigin, query, toolSearch);
  }, [entries, origin, query, shelf, toolSearch]);
  const page = useVisibleItems(filtered, 50);

  if (isLoading) return <LoadingRows rows={4} />;
  if (error) return <ErrorState error={error} onRetry={onRetry} />;

  return (
    <div className="min-h-[620px] overflow-hidden rounded-lg border bg-background">
      <CatalogueTabs connected={connected} total={entries.length} />
      <div className="flex min-h-[560px] overflow-hidden">
        <CatalogueRail
          counts={shelves(entries)}
          connected={connected}
          chosen={shelf}
          onChoose={setShelf}
        />

        <section className="flex min-w-[28rem] flex-1 flex-col overflow-hidden">
          <div className="flex flex-wrap items-center gap-2 border-b px-4 py-3">
            <div className="flex h-8 min-w-[14rem] flex-1 items-center gap-2 rounded-md border bg-card px-2.5 sm:max-w-72">
              <Plug className="size-4 text-muted-foreground" aria-hidden />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("mcp.searchServers")}
                aria-label={t("mcp.searchServers")}
                className="h-7 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
              />
            </div>
            <OriginFilter value={origin} onChange={setOrigin} />
            <span className="ml-auto min-w-0 truncate text-xs text-muted-foreground">
              {t("mcp.catalogueSummary", {
                shown: filtered.length,
                total: entries.length,
                connected,
              })}
            </span>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto p-4">
            {filtered.length === 0 ? (
              <EmptyState
                icon={<Plug className="size-6" />}
                title={t("mcp.nothingHere")}
                hint={t("mcp.nothingHereHint")}
              />
            ) : (
              <>
                <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,268px),1fr))] gap-3">
                  {page.visible.map((entry) => (
                    <CatalogueCard
                      key={entry.name}
                      entry={entry}
                      selected={panel === entry.name}
                      onOpen={() => setPanel(entry.name)}
                    />
                  ))}
                </div>
                <LoadMore
                  loaded={page.loaded}
                  total={page.total}
                  hasMore={page.hasMore}
                  isLoading={false}
                  onLoad={page.loadMore}
                />
              </>
            )}
          </div>
        </section>

        {chosenEntry && chosenServer ? (
          <ConnectedServerPanel
            entry={chosenEntry}
            server={chosenServer}
            tools={toolsByServer.get(chosenEntry.name) ?? []}
            onClose={() => setPanel(undefined)}
            onConfigure={() =>
              void navigate(`/integrations/mcp/${chosenEntry.name}`)
            }
          />
        ) : (
          <ConnectServerPanel
            key={panel === undefined ? "closed" : (panel ?? "custom")}
            open={panel !== undefined}
            recipe={chosenRecipe}
            entry={chosenEntry}
            onClose={() => setPanel(undefined)}
            onConnected={(name) => void navigate(`/integrations/mcp/${name}`)}
            onCustom={() => setPanel(null)}
          />
        )}
      </div>
    </div>
  );
}

function CatalogueTabs({
  connected,
  total,
}: {
  connected: number;
  total: number;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex h-11 items-center gap-5 border-b bg-card px-4">
      <Link
        to="/integrations"
        className="inline-flex h-11 items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        {t("integrations.connected")}
        <span className="rounded bg-muted px-1.5 font-mono text-2xs">
          {connected}
        </span>
      </Link>
      <span className="inline-flex h-11 items-center gap-2 border-b-2 border-primary text-sm font-medium">
        {t("mcp.catalogueTab")}
        <span className="rounded bg-primary/10 px-1.5 font-mono text-2xs text-primary">
          {total}
        </span>
      </span>
      <span className="inline-flex h-11 items-center text-sm text-muted-foreground">
        {t("mcp.syncTab")}
      </span>
      <span className="ml-auto hidden text-xs text-muted-foreground lg:block">
        {t("mcp.catalogueTabHint")}
      </span>
    </div>
  );
}

function OriginFilter({
  value,
  onChange,
}: {
  value: OriginFilter;
  onChange: (value: OriginFilter) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex h-8 items-center gap-1 rounded-md border bg-muted p-1">
      {ORIGINS.map((one) => (
        <button
          key={one}
          type="button"
          onClick={() => onChange(one)}
          className={cn(
            "h-6 rounded px-2 text-xs text-muted-foreground",
            value === one && "bg-card font-medium text-foreground shadow-xs",
          )}
        >
          {t(`mcp.originFilter.${one}`)}
        </button>
      ))}
    </div>
  );
}

function ConnectedServerPanel({
  entry,
  server,
  tools,
  onClose,
  onConfigure,
}: {
  entry: Listing;
  server: MCPServer;
  tools: Tool[];
  onClose: () => void;
  onConfigure: () => void;
}) {
  const { t } = useTranslation();
  const state = stateOf(server.enabled, server.health);
  const exposed = exposedTools(server, tools);
  const writeCount = exposed.filter((tool) => actsOnTheWorld(tool)).length;

  return (
    <aside className="flex w-[376px] shrink-0 flex-col overflow-hidden border-l bg-card">
      <PanelHeader entry={entry} onClose={onClose} />
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <p className="text-sm text-muted-foreground">
          {entry.description ?? t("mcp.connectedWithoutRecipe")}
        </p>

        <div className="space-y-2">
          <Eyebrow>{t("mcp.connection")}</Eyebrow>
          <div className="grid grid-cols-2 gap-1 rounded-md border bg-muted p-1 text-xs">
            <span
              className={cn(
                "rounded px-2 py-1 text-center",
                server.transport === "http" && "bg-card font-medium shadow-xs",
              )}
            >
              {t("integrations.transportHTTP")}
            </span>
            <span
              className={cn(
                "rounded px-2 py-1 text-center",
                server.transport === "stdio" && "bg-card font-medium shadow-xs",
              )}
            >
              {t("integrations.transportStdio")}
            </span>
          </div>
          <div className="flex items-center gap-2 rounded-md border bg-muted px-2 py-2">
            <LinkIcon className="size-4 shrink-0 text-muted-foreground" />
            <Mono className="truncate text-xs">{endpointOf(server, entry)}</Mono>
          </div>
          <div className="flex items-center gap-2 text-sm">
            <KeyRound className="size-4 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate">
              {entry.auth ?? t("mcp.credentialManaged")}
            </span>
            <span className={cn("rounded px-2 py-1 text-2xs", state.pill)}>
              {t(state.label)}
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            <AuthModeBadges modes={entry.authModes} />
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-baseline gap-2">
            <Eyebrow>{t("mcp.exposedTools")}</Eyebrow>
            <span className="text-xs text-muted-foreground">
              {t("mcp.exposedToolsSummary", {
                chosen: exposed.length,
                total: tools.length,
                write: writeCount,
              })}
            </span>
          </div>
          {tools.length === 0 ? (
            <p className="rounded-lg border p-3 text-xs text-muted-foreground">
              {t("mcp.noPublishedTools")}
            </p>
          ) : (
            <ul className="divide-y overflow-hidden rounded-lg border">
              {tools.map((tool) => {
                const on = exposed.includes(tool);
                return (
                  <li key={tool.toolId} className="flex items-start gap-2 p-2">
                    <span
                      className={cn(
                        "mt-0.5 grid size-4 shrink-0 place-items-center rounded border text-[10px]",
                        on
                          ? "border-primary bg-primary text-primary-foreground"
                          : "border-border-strong bg-card",
                      )}
                    >
                      {on ? "✓" : ""}
                    </span>
                    <div className="min-w-0 flex-1">
                      <Mono className="block truncate text-xs">
                        {tool.toolId}
                      </Mono>
                      {tool.description && (
                        <p className="line-clamp-2 text-2xs text-muted-foreground">
                          {tool.description}
                        </p>
                      )}
                    </div>
                    <EffectBadge effect={tool.effect} stale={tool.stale} />
                  </li>
                );
              })}
            </ul>
          )}
        </div>

        <div className="flex gap-2 rounded-lg border bg-muted p-3">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div className="space-y-1">
            <p className="text-xs font-medium">
              {writeCount > 0
                ? t("mcp.governanceWriteTitle")
                : t("mcp.governanceReadTitle")}
            </p>
            <p className="text-xs text-muted-foreground">
              {writeCount > 0
                ? t("mcp.governanceWriteBody", { count: writeCount })
                : t("mcp.governanceReadBody")}
            </p>
          </div>
        </div>
      </div>
      <div className="flex items-center gap-2 border-t p-4">
        <span className="flex-1 text-xs text-muted-foreground">
          {t("mcp.connectedFooter")}
        </span>
        <Button onClick={onConfigure}>{t("mcp.configure")}</Button>
      </div>
    </aside>
  );
}

function ConnectServerPanel({
  open,
  recipe,
  entry,
  onClose,
  onConnected,
  onCustom,
}: {
  open: boolean;
  recipe: ServerRecipe | null;
  entry: Listing | null;
  onClose: () => void;
  onConnected: (name: string) => void;
  onCustom: () => void;
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

  if (!open) {
    return (
      <aside className="flex w-[376px] shrink-0 flex-col border-l bg-card p-4">
        <EmptyState
          icon={<Server className="size-6" />}
          title={t("mcp.chooseServer")}
          hint={t("mcp.chooseServerHint")}
        />
        <Button variant="outline" className="mt-4" onClick={onCustom}>
          <Plus className="size-4" aria-hidden />
          {t("mcp.connectByAddress")}
        </Button>
      </aside>
    );
  }

  return (
    <aside className="flex w-[376px] shrink-0 flex-col overflow-hidden border-l bg-card">
      <PanelHeader
        entry={
          entry ?? {
            name: "custom",
            title: t("mcp.customServer"),
            category: "operations",
            publisher: null,
            description: null,
            configured: false,
            enabled: false,
            health: null,
            tools: null,
            status: null,
            configRequirements: [],
            authModes: [],
            auth: null,
            transport: null,
            command: null,
            args: [],
            url: null,
            recipe: null,
          }
        }
        onClose={onClose}
      />

      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {recipe?.note ?? t("mcp.connectPanelHint")}
          </p>
          {recipe && (
            <div className="flex flex-wrap items-center gap-2 text-2xs text-muted-foreground">
              <RecipeStatusBadge status={recipe.status} />
              <ConfigRequirementBadges requirements={recipe.configRequirements} />
              <AuthModeBadges modes={recipe.authModes ?? []} />
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
            id="mcp-connect-form"
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <ServerFormBody
              form={form}
              editing={false}
              hasSecret={false}
              hasConfigFile={false}
            />
          </form>
        </Form>
      </div>

      <div className="flex items-center gap-2 border-t p-4">
        <span className="flex-1 text-xs text-muted-foreground">
          {t("mcp.connectFooter")}
        </span>
        <Button type="submit" form="mcp-connect-form" disabled={saving}>
          {t("integrations.connect")}
        </Button>
      </div>
    </aside>
  );
}

function PanelHeader({
  entry,
  onClose,
}: {
  entry: Pick<Listing, "name" | "title" | "category" | "publisher" | "status">;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <header className="flex items-center gap-3 border-b p-4">
      <CatalogueIcon entry={entry} className="size-8 rounded-md" />
      <div className="min-w-0 flex-1">
        <h2 className="truncate text-sm font-medium">{entry.title}</h2>
        <p className="truncate text-xs text-muted-foreground">
          {entry.publisher
            ? t("mcp.publisherLine", { publisher: entry.publisher })
            : t("mcp.publisherUnknown")}
        </p>
      </div>
      {entry.status && <RecipeStatusBadge status={entry.status} />}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7"
        onClick={onClose}
        aria-label={t("common.close")}
      >
        <X className="size-4" aria-hidden />
      </Button>
    </header>
  );
}

function Eyebrow({ children }: { children: string }) {
  return (
    <span className="text-2xs uppercase tracking-label text-muted-foreground">
      {children}
    </span>
  );
}

function groupTools(tools: Tool[]) {
  const byServer = new Map<string, Tool[]>();
  for (const tool of tools) {
    const server = tool.server ?? "";
    byServer.set(server, [...(byServer.get(server) ?? []), tool]);
  }
  return byServer;
}

function toolSearchTerms(tools: Tool[]) {
  const byServer = new Map<string, string[]>();
  for (const tool of tools) {
    const server = tool.server ?? "";
    byServer.set(server, [
      ...(byServer.get(server) ?? []),
      tool.toolId,
      tool.description ?? "",
      remoteNameOf(tool),
    ]);
  }
  return byServer;
}

function exposedTools(server: MCPServer, tools: Tool[]) {
  if (!server.surface) return tools;
  const surface = new Set(server.surface);
  return tools.filter((tool) => surface.has(remoteNameOf(tool)));
}

function actsOnTheWorld(tool: Tool) {
  return tool.effect !== "read" && tool.effect !== "unknown";
}

function endpointOf(server: MCPServer, entry: Listing) {
  if (server.transport === "http") return server.url ?? entry.url ?? "";
  return [server.command ?? entry.command ?? "", ...(server.args ?? entry.args)]
    .filter(Boolean)
    .join(" ");
}
