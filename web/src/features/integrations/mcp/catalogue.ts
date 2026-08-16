import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

/**
 * One shelf holds two kinds of thing, and the reader is asking one question.
 *
 * "What can this installation reach?" is answered by the servers it is
 * connected to and the ones it knows how to connect, and separating them into
 * two lists makes somebody look in both to find out that a server they already
 * run is already running.
 *
 * So they are one list, and connectedness is a state on the entry rather than
 * a section heading. A connected server the platform has no recipe for is
 * still here — the installation talks to it, which is the more important fact
 * than whether we happen to have read about it.
 */
export type Listing = {
  name: string;
  title: string;
  category: string;
  publisher: string | null;
  description: string | null;
  connected: boolean;
  /** How many tools it offers now. Null when nothing has reached it. */
  tools: number | null;
  recipe: ServerRecipe | null;
};

const UNSHELVED = "operations";

export function listing(
  servers: MCPServer[],
  recipes: ServerRecipe[],
  toolCounts: Record<string, number>,
): Listing[] {
  const byName = new Map<string, Listing>();

  for (const recipe of recipes) {
    byName.set(recipe.server, {
      name: recipe.server,
      title: recipe.title,
      category: recipe.category,
      publisher: recipe.publisher,
      description: recipe.note ?? null,
      connected: false,
      tools: null,
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
      connected: true,
      tools: toolCounts[server.name] ?? 0,
      recipe: known?.recipe ?? null,
    });
  }

  return [...byName.values()].sort((a, b) => {
    // What is running comes first. It is what somebody is here to check.
    if (a.connected !== b.connected) return a.connected ? -1 : 1;
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
