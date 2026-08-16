import { describe, expect, it } from "vitest";
import { listing, matching, shelves } from "@/features/integrations/mcp/catalogue";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

const recipe = (server: string, category: string): ServerRecipe => ({
  server,
  title: server.toUpperCase(),
  category,
  publisher: "Somebody Else",
  docsFrom: "publisher",
  provenance: "documentation",
});

const connected = (name: string): MCPServer => ({ name, enabled: true });

describe("what the catalogue shows", () => {
  /*
   * One list, because "what can we reach" is one question. Two lists make
   * somebody check both to find that a server they already run is running.
   */
  it("merges a connected server with the recipe for it, rather than listing it twice", () => {
    const shown = listing([connected("github")], [recipe("github", "code")], {
      github: 12,
    });
    expect(shown).toHaveLength(1);
    expect(shown[0]).toMatchObject({ connected: true, tools: 12, title: "GITHUB" });
  });

  // The installation talks to it, which matters more than whether we happen
  // to have read about it.
  it("keeps a connected server nobody wrote a recipe for", () => {
    const shown = listing([connected("in-house")], [], {});
    expect(shown.map((s) => s.name)).toEqual(["in-house"]);
    expect(shown[0]?.publisher).toBeNull();
  });

  it("puts what is running first, because that is what somebody came to check", () => {
    const shown = listing(
      [connected("zzz")],
      [recipe("aaa", "code"), recipe("zzz", "code")],
      {},
    );
    expect(shown[0]?.name).toBe("zzz");
  });

  it("counts each shelf, so an empty one says so before it is clicked", () => {
    const shown = listing([], [recipe("a", "code"), recipe("b", "data"), recipe("c", "code")], {});
    expect(shelves(shown)).toEqual([
      { category: "code", count: 2 },
      { category: "data", count: 1 },
    ]);
  });

  it("searches the publisher too, which is how somebody looks for a vendor", () => {
    const shown = listing([], [recipe("x", "code")], {});
    expect(matching(shown, "somebody")).toHaveLength(1);
    expect(matching(shown, "nobody")).toHaveLength(0);
  });
});
