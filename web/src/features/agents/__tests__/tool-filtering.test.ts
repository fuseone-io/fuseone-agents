import { describe, expect, it } from "vitest";
import {
  ENABLED,
  byFilter,
  inNav,
  navFor,
  tally,
} from "@/features/agents/tool-filtering";
import type { Policy, Tool } from "@/lib/api/client";

function tool(toolId: string, effect: Tool["effect"] = "read"): Tool {
  return {
    toolId,
    server: toolId.split(".")[0] ?? "",
    effect,
    untrusted: false,
  } as Tool;
}

const CATALOGUE = [
  tool("crm.lookup"),
  tool("crm.reply", "write"),
  tool("erp.balance"),
  tool("erp.transfer", "financial"),
];

const DENY_TRANSFER: Policy[] = [
  {
    code: "POL-114",
    name: "no transfers",
    effect: "deny",
    mode: "enforce",
    enabled: true,
    resource: "erp.transfer",
  },
];

describe("narrowing the catalogue", () => {
  it("separates what only reads from what changes something", () => {
    expect(byFilter(CATALOGUE, "read", []).map((t) => t.toolId)).toEqual([
      "crm.lookup",
      "erp.balance",
    ]);
    expect(byFilter(CATALOGUE, "write", []).map((t) => t.toolId)).toEqual([
      "crm.reply",
      "erp.transfer",
    ]);
  });

  it("finds what will stop and wait for a person", () => {
    // Not an effect but the Gate's answer, and the one somebody scanning a
    // long catalogue is actually deciding about.
    expect(byFilter(CATALOGUE, "asks", []).map((t) => t.toolId)).toEqual([
      "crm.reply",
    ]);
  });
});

describe("the catalogue's own navigation", () => {
  it("counts everything, then what this agent reaches, then each server", () => {
    const nav = navFor(CATALOGUE, ["crm.reply"], {
      all: "All",
      enabled: "Enabled",
    });

    expect(nav.map((n) => [n.label, n.count])).toEqual([
      ["All", 4],
      ["Enabled", 1],
      ["crm", 2],
      ["erp", 2],
    ]);
  });

  it("narrows to one server, or to what is already granted", () => {
    expect(inNav(CATALOGUE, "erp", []).map((t) => t.toolId)).toEqual([
      "erp.balance",
      "erp.transfer",
    ]);
    expect(
      inNav(CATALOGUE, ENABLED, ["crm.lookup"]).map((t) => t.toolId),
    ).toEqual(["crm.lookup"]);
    expect(inNav(CATALOGUE, "", []).length).toBe(4);
  });
});

describe("what the footer counts", () => {
  it("says how many are enabled, how many write, and how many are denied", () => {
    // The denied count earns its line: a tool ticked here that a policy
    // refuses is not enabled, and an author who misses that has built an agent
    // whose first attempt at that call fails.
    expect(
      tally(
        CATALOGUE,
        ["crm.lookup", "crm.reply", "erp.transfer"],
        DENY_TRANSFER,
      ),
    ).toEqual({ enabled: 3, writing: 2, denied: 1 });
  });

  it("counts nothing when nothing is chosen", () => {
    expect(tally(CATALOGUE, [], [])).toEqual({
      enabled: 0,
      writing: 0,
      denied: 0,
    });
  });
});
