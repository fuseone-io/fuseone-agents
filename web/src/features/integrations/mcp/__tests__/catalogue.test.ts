import { describe, expect, it } from "vitest";
import {
  availableEntries,
  listing,
  matching,
  shelves,
} from "@/features/integrations/mcp/catalogue";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

const recipe = (server: string, category: string): ServerRecipe => ({
  server,
  title: server.toUpperCase(),
  category,
  publisher: "Somebody Else",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "published",
  configRequirements: [],
  requiresPersonalCredential: false,
});

const connected = (name: string, tools = 0): MCPServer => ({
  name,
  enabled: true,
  health: { reachable: true, toolCount: tools, observedAt: "2026-08-16T00:00:00Z" },
});

describe("what the catalogue shows", () => {
  /*
   * One join, because a server that is both known and configured is still one
   * system. The side rail may split connected from available, but the split
   * cannot duplicate a thing the installation already runs.
   */
  it("merges a connected server with the recipe for it, rather than listing it twice", () => {
    const shown = listing([connected("github", 12)], [recipe("github", "code")]);
    expect(shown).toHaveLength(1);
    expect(shown[0]).toMatchObject({
      configured: true,
      tools: 12,
      title: "GITHUB",
      status: "published",
      configRequirements: [],
    });
  });

  // The installation talks to it, which matters more than whether we happen
  // to have read about it.
  it("keeps a connected server nobody wrote a recipe for", () => {
    const shown = listing([connected("in-house")], []);
    expect(shown.map((s) => s.name)).toEqual(["in-house"]);
    expect(shown[0]?.publisher).toBeNull();
    expect(shown[0]?.status).toBeNull();
  });

  it("puts what is running first, because that is what somebody came to check", () => {
    const shown = listing(
      [connected("zzz")],
      [recipe("aaa", "code"), recipe("zzz", "code")],
    );
    expect(shown[0]?.name).toBe("zzz");
  });

  it("counts each shelf, so an empty one says so before it is clicked", () => {
    const shown = listing([], [recipe("a", "code"), recipe("b", "data"), recipe("c", "code")]);
    expect(shelves(shown)).toEqual([
      { category: "code", count: 2 },
      { category: "data", count: 1 },
    ]);
  });

  it("searches the publisher too, which is how somebody looks for a vendor", () => {
    const shown = listing([], [recipe("x", "code")]);
    expect(matching(shown, "somebody")).toHaveLength(1);
    expect(matching(shown, "nobody")).toHaveLength(0);
  });

  it("keeps already configured servers out of the available shelf", () => {
    const shown = listing(
      [connected("github", 12)],
      [recipe("github", "code"), recipe("stripe", "finance")],
    );
    expect(availableEntries(shown).map((one) => one.name)).toEqual(["stripe"]);
  });
});

/*
A configured server is not a working one.

Four states hide behind "connected": switched off, never reached, reached and
refusing, answering. The card used to draw all four as running, so a server
somebody had switched off looked as healthy as one serving traffic.
*/
it("carries what the card needs to tell a switched-off server from a working one", () => {
  const off: MCPServer = { name: "paused", enabled: false };
  const broken: MCPServer = {
    name: "broken",
    enabled: true,
    health: { reachable: false, toolCount: 0, observedAt: "2026-08-16T00:00:00Z" },
  };

  const shown = listing([off, broken], []);
  const by = Object.fromEntries(shown.map((one) => [one.name, one]));
  expect(by.paused?.enabled).toBe(false);
  expect(by.broken?.health?.reachable).toBe(false);
  // The count comes from the observation, so an unreachable server offers
  // nothing rather than whatever the catalogue still remembers.
  expect(by.broken?.tools).toBe(0);
  expect(by.paused?.tools).toBeNull();
});
