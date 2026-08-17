import type { IntegrationHealth, MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

/**
 * A joined read-model for recipes and configured servers.
 *
 * The page now splits connected from available, but the join still has to be
 * one rule: a recipe for a server that is already configured is the same
 * thing wearing catalogue context, not a second thing the operator can connect
 * again. A configured server with no recipe still survives the join because
 * the installation talks to it, which matters more than whether we wrote a
 * recipe for it.
 */
export type Listing = {
  name: string;
  title: string;
  category: string;
  publisher: string | null;
  description: string | null;
  /** Configured here at all. Not the same as answering, or even switched on. */
  configured: boolean;
  enabled: boolean;
  /*
    What was observed the last time anybody tried, and null when nobody has.

    Four states hide behind "connected" — off, never reached, refusing,
    answering — and the card has to tell them apart. Collapsing them drew a
    switched-off server in the colour of a healthy one.
  */
  health: IntegrationHealth | null;
  /*
    How many tools it offers *now*, from the observation rather than from the
    catalogue.

    The catalogue is what this installation has ever been offered and never
    shrinks, so counting it reports tools a server stopped offering last week.
  */
  tools: number | null;
  status: ServerRecipe["status"] | null;
  configRequirements: ServerRecipe["configRequirements"];
  recipe: ServerRecipe | null;
};

const UNSHELVED = "operations";

export function listing(servers: MCPServer[], recipes: ServerRecipe[]): Listing[] {
  const byName = new Map<string, Listing>();

  for (const recipe of recipes) {
    byName.set(recipe.server, {
      name: recipe.server,
      title: recipe.title,
      category: recipe.category,
      publisher: recipe.publisher,
      description: recipe.note ?? null,
      configured: false,
      enabled: false,
      health: null,
      tools: null,
      status: recipe.status,
      configRequirements: recipe.configRequirements,
      recipe,
    });
  }

  for (const server of servers) {
    const known = byName.get(server.name);
    byName.set(server.name, {
      name: server.name,
      title: known?.title ?? server.name,
      // A server nobody wrote a recipe for still needs a shelf, or it falls
      // off every filtered view including the one somebody uses by default.
      category: known?.category ?? UNSHELVED,
      publisher: known?.publisher ?? null,
      description: known?.description ?? null,
      configured: true,
      enabled: server.enabled,
      health: server.health ?? null,
      tools: server.health?.toolCount ?? null,
      status: known?.status ?? null,
      configRequirements: known?.configRequirements ?? [],
      recipe: known?.recipe ?? null,
    });
  }

  return [...byName.values()].sort((a, b) => {
    // What the installation runs comes first. It is what somebody is here to
    // check, whether or not it is answering — a broken one most of all.
    if (a.configured !== b.configured) return a.configured ? -1 : 1;
    return a.title.localeCompare(b.title);
  });
}

export function shelves(entries: Listing[]): { category: string; count: number }[] {
  const counts = new Map<string, number>();
  for (const one of entries) {
    counts.set(one.category, (counts.get(one.category) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([category, count]) => ({ category, count }))
    .sort((a, b) => a.category.localeCompare(b.category));
}

export function matching(entries: Listing[], query: string): Listing[] {
  const needle = query.trim().toLowerCase();
  if (needle === "") return entries;
  return entries.filter((one) =>
    [one.name, one.title, one.publisher ?? ""].some((field) =>
      field.toLowerCase().includes(needle),
    ),
  );
}

export function availableEntries(entries: Listing[]): Listing[] {
  return entries.filter((one) => !one.configured);
}
