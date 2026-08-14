import { describe, expect, it } from "vitest";
import { segments } from "@/features/agents/instruction-entities";
import type { Policy, Tool } from "@/lib/api/client";

/*
What the prose talks about, found in the prose itself.

Nothing is stored beside the text, which is what lets a pasted paragraph light
up exactly as a typed one does — and what stops a span model drifting from the
words it claims to describe.
*/

const CATALOGUE: Tool[] = [
  { toolId: "crm.lookup", server: "crm", effect: "read", untrusted: true },
  { toolId: "erp.refund", server: "erp", effect: "financial", untrusted: true },
];

const DENIES: Policy[] = [
  {
    code: "POL-114",
    name: "Sem estorno automático",
    resource: "erp.refund",
    effect: "deny",
    // Enforcing rather than observing: a rule somebody is only watching
    // does not refuse anything, so the prose is not promising the impossible.
    mode: "enforce",
    enabled: true,
  } as unknown as Policy,
];

describe("what an instruction names", () => {
  it("finds a tool the agent may call", () => {
    expect(
      segments("Use crm.lookup para achar o cliente.", CATALOGUE, []),
    ).toEqual([
      { kind: "text", text: "Use " },
      { kind: "tool", text: "crm.lookup" },
      { kind: "text", text: " para achar o cliente." },
    ]);
  });

  it("marks a tool the policy in force denies", () => {
    // The one thing worth marking in prose: the sentence promises something
    // that will not happen, and the next person to read it finds out.
    const found = segments("Se precisar, use erp.refund.", CATALOGUE, DENIES);

    expect(found.find((s) => s.text === "erp.refund")?.kind).toBe("denied");
  });

  it("finds money written the way a person writes it", () => {
    const found = segments("Pare se passar de R$ 500.", CATALOGUE, []);

    expect(found.find((s) => s.kind === "limit")?.text).toBe("R$ 500");
  });

  it("says nothing about an amount spelled out in words", () => {
    // Narrow on purpose: a highlight that is wrong once is one nobody reads
    // again, and "quinhentos reais" is a phrase, not a limit anybody set.
    expect(
      segments("Pare se passar de quinhentos reais.", CATALOGUE, []),
    ).toEqual([{ kind: "text", text: "Pare se passar de quinhentos reais." }]);
  });

  it("never matches half of a longer identifier", () => {
    const catalogue: Tool[] = [
      { toolId: "crm.look", server: "crm", effect: "read", untrusted: true },
      ...CATALOGUE,
    ];

    expect(
      segments("crm.lookup", catalogue, []).map((s) => s.text),
    ).toEqual(["crm.lookup"]);
  });
});
