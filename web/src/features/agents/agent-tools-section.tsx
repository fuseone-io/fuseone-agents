import { Plug, Search, Wrench } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Section } from "@/features/policies/section";
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

  return (
    <Section
      icon={Wrench}
      title={t("admin.tools")}
      hint={t("agents.enabledOfTotal", {
        enabled: counted.enabled,
        total: catalogue.length,
      })}
      action={
        <Button variant="outline" size="sm" asChild>
          <Link to="/integrations">
            <Plug className="size-3.5" aria-hidden />
            {t("agents.connectIntegration")}
          </Link>
        </Button>
      }
      flush
    >
      {catalogue.length === 0 ? (
        <p className="p-4 text-xs text-muted-foreground">
          {t("agents.emptyCatalogue")}
        </p>
      ) : (
        <div className="grid min-h-0 sm:grid-cols-[minmax(0,206px)_minmax(0,1fr)]">
          <ToolCatalogueNav
            entries={nav}
            chosen={server}
            onChoose={setServer}
          />

          <div className="flex min-w-0 flex-col">
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
                  screen and belongs beside it rather than in the header. */}
              <ToggleGroup
                type="single"
                size="sm"
                value={filter}
                onValueChange={(next) => next && setFilter(next as ToolFilter)}
              >
                {FILTERS.map((option) => (
                  <ToggleGroupItem
                    key={option}
                    value={option}
                    className="px-2.5 text-xs"
                  >
                    {t(`agents.filter.${option}`)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>

            {/* The list scrolls inside the card rather than making it as tall
                as the catalogue. An installation with a hundred and twenty
                tools would otherwise push everything below it — the ceilings,
                the reading, the button — off the screen. */}
            <ScrollArea className="h-[22rem]">
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

            <div className="mt-auto flex flex-wrap items-baseline gap-x-3 gap-y-1 border-t border-border p-3">
              <p className="text-xs">{t("agents.toolTally", counted)}</p>
              <p className="min-w-0 flex-1 text-2xs text-muted-foreground">
                {t("agents.rightColumnIsPolicy")}
              </p>
            </div>
          </div>
        </div>
      )}
    </Section>
  );
}
