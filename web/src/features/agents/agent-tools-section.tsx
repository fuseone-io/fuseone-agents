import { Eye, List, Pencil, Plug, Search, UserRoundCheck } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToolCatalogueNav } from "@/features/agents/tool-catalogue-nav";
import { ToolGroupRows } from "@/features/agents/tool-group-rows";
import { grouped, matching } from "@/features/agents/tool-catalogue";
import {
  byFilter,
  inNav,
  navFor,
  tally,
  type ToolFilter,
} from "@/features/agents/tool-filtering";
import type { Policy, Tool } from "@/lib/api/client";

const FILTERS: ToolFilter[] = ["all", "read", "write", "asks"];
const FILTER_ICONS = {
  all: List,
  read: Eye,
  write: Pencil,
  asks: UserRoundCheck,
} satisfies Record<ToolFilter, typeof Search>;

/**
 * What this agent may call, and what will happen when it does.
 *
 * Two panes, because the card holds two different things: the catalogue is the
 * organisation's and the choice is this agent's. The right-hand column of each
 * row is derived from the policies in force rather than set here — making it a
 * per-agent setting would be a fourth place that decides whether a human is
 * asked, beside the ladder, the policies and the taint rules.
 */
export function AgentToolsSection({
  catalogue,
  policies,
  granted,
  patch,
}: {
  catalogue: Tool[];
  policies: Policy[];
  granted: string[];
  patch: (over: { tools: string[] }) => void;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ToolFilter>("all");
  const [server, setServer] = useState("");

  const toggle = (tool: string) =>
    patch({
      tools: granted.includes(tool)
        ? granted.filter((t) => t !== tool)
        : [...granted, tool],
    });

  const nav = navFor(catalogue, granted, {
    all: t("agents.allTools"),
    enabled: t("agents.enabledHere"),
  });
  const shown = byFilter(
    matching(inNav(catalogue, server, granted), query),
    filter,
    policies,
  );
  const groups = grouped(shown, granted);
  const counted = tally(catalogue, granted, policies);

  if (catalogue.length === 0) {
    return (
      <p className="p-5 text-xs text-muted-foreground">
        {t("agents.emptyCatalogue")}
      </p>
    );
  }

  return (
    /*
    The catalogue fills its tab rather than sitting in a card.
    
    Browsing is not reading a form: a short window scrolling inside a page
    that also scrolls is two scrollbars for one list, and the second one is
    always the wrong one.
    */
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-5 py-3">
        <span className="text-xs text-muted-foreground">
          {t("agents.enabledOfTotal", {
            enabled: counted.enabled,
            total: catalogue.length,
          })}
        </span>
        <Button variant="outline" size="sm" asChild className="ml-auto h-8">
          <Link to="/integrations">
            <Plug className="size-3.5" aria-hidden />
            {t("agents.connectIntegration")}
          </Link>
        </Button>
      </div>

      <div className="grid min-h-0 flex-1 overflow-hidden sm:grid-cols-[minmax(0,206px)_minmax(0,1fr)]">
        <ToolCatalogueNav entries={nav} chosen={server} onChoose={setServer} />

        {/* min-h-0, or the column refuses to shrink below its content and
            pushes the tally out of the card and under the footer. */}
        <div className="flex min-h-0 min-w-0 flex-col">
          <div className="flex flex-wrap items-center gap-2 border-b border-border p-3">
            <div className="relative min-w-48 flex-1">
              <Search
                className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
                aria-hidden
              />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("agents.searchTools")}
                aria-label={t("agents.searchTools")}
                className="h-8 pl-8"
              />
            </div>

            {/* A filter, not navigation: it narrows what is already on the
                  screen and belongs beside it rather than in the header. Drawn
                  as a tab track because that is what the design system calls
                  this shape, and its tokens are literally the tab ones. */}
            <Tabs
              value={filter}
              onValueChange={(next) => setFilter(next as ToolFilter)}
            >
              <TabsList>
                {FILTERS.map((option) => {
                  const Icon = FILTER_ICONS[option];
                  return (
                    <TabsTrigger key={option} value={option}>
                      <Icon aria-hidden />
                      {t(`agents.filter.${option}`)}
                    </TabsTrigger>
                  );
                })}
              </TabsList>
            </Tabs>
          </div>

          {/* A ceiling rather than a height: past it the list scrolls, and
                short of it the row is as tall as the taller pane — which is
                what stops the navigation's own note being clipped against the
                card's edge when the catalogue is small. An installation with a
                hundred and twenty tools would otherwise push everything below
                it — the ceilings,
                the reading, the button — off the screen. */}
          <ScrollArea className="min-h-0 flex-1">
            <div className="flex flex-col px-3">
              {groups.length === 0 ? (
                <p className="py-4 text-xs text-muted-foreground">
                  {t("agents.noToolMatches")}
                </p>
              ) : (
                groups.map((group) => (
                  <ToolGroupRows
                    key={group.server}
                    group={group}
                    granted={granted}
                    policies={policies}
                    onToggle={toggle}
                  />
                ))
              )}
            </div>
          </ScrollArea>
        </div>
      </div>

      {/* Under both panes rather than inside the right one. Sharing a row with
          the navigation coupled their heights, and the taller of the two
          clipped the other against the edge. */}
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 border-t border-border px-5 py-3">
        <p className="text-xs">{t("agents.toolTally", counted)}</p>
        <p className="min-w-0 flex-1 text-2xs text-muted-foreground">
          {t("agents.rightColumnIsPolicy")}
        </p>
      </div>
    </div>
  );
}
