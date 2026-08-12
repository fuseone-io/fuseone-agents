import { describe, expect, it } from "vitest";
import { ruleFor } from "@/features/agents/tool-rule";
import { riskSurface } from "@/features/agents/risk-surface";
import type { Policy } from "@/lib/api/client";

function policy(over: Partial<Policy>): Policy {
  return {
    code: "POL-1",
    name: "x",
    sentence: "x",
    effect: "deny",
    mode: "enforce",
    enabled: true,
    resource: "*",
    ...over,
  };
}

describe("what a tool will actually do", () => {
  it("falls back to the built-in ladder when no policy covers it", () => {
    // The ladder is the floor, and a screen that showed nothing where no
    // policy exists would suggest a write just happens.
    expect(ruleFor("crm.lookup", "read", []).kind).toBe("allowed");
    expect(ruleFor("crm.reply", "write", []).kind).toBe("asks");
    expect(ruleFor("erp.transfer", "financial", []).kind).toBe("blocked");
  });

  it("names the policy that changed the answer", () => {
    // Changing this means editing that policy, so the column has to say which
    // one — otherwise the reader has nowhere to go.
    const rule = ruleFor("crm.reply", "write", [
      policy({ code: "POL-114", resource: "crm.*", effect: "deny" }),
    ]);

    expect(rule.kind).toBe("blocked");
    expect(rule.because).toBe("POL-114");
  });

  it("lets an explicit allow lower the built-in floor", () => {
    const rule = ruleFor("crm.reply", "write", [
      policy({ code: "POL-020", resource: "crm.reply", effect: "allow" }),
    ]);

    expect(rule.kind).toBe("allowed");
    expect(rule.because).toBe("POL-020");
  });

  it("takes the most restrictive when two policies cover the same tool", () => {
    const rule = ruleFor("crm.reply", "write", [
      policy({ code: "POL-020", resource: "crm.*", effect: "allow" }),
      policy({ code: "POL-114", resource: "crm.reply", effect: "deny" }),
    ]);

    expect(rule.kind).toBe("blocked");
  });

  it("says a conditional rule only applies sometimes", () => {
    // A policy firing when args.rows > 100 does something to some calls. A
    // column claiming it happens on every one is worse than saying nothing.
    const rule = ruleFor("crm.reply", "write", [
      policy({
        code: "POL-114",
        resource: "crm.*",
        effect: "deny",
        conditions: [{ field: "args.rows", op: "gt", value: "100" }],
      }),
    ]);

    expect(rule.kind).toBe("asks");
    expect(rule.label).toContain("às vezes");
  });

  it("ignores a policy that is only watching", () => {
    // A monitored rule changes no verdict, so a column reporting it as the
    // outcome would describe something that does not happen.
    const rule = ruleFor("crm.reply", "write", [
      policy({
        code: "POL-114",
        resource: "crm.*",
        effect: "deny",
        mode: "monitor",
      }),
    ]);

    expect(rule.kind).toBe("asks");
    expect(rule.because).toBeUndefined();
  });

  it("ignores a policy that is switched off", () => {
    const rule = ruleFor("crm.reply", "write", [
      policy({
        code: "POL-114",
        resource: "crm.*",
        effect: "deny",
        enabled: false,
      }),
    ]);

    expect(rule.kind).toBe("asks");
  });
});

describe("the risk surface", () => {
  const catalogue = [
    {
      toolId: "crm.lookup",
      server: "crm",
      effect: "read" as const,
      untrusted: true,
    },
    {
      toolId: "crm.reply",
      server: "crm",
      effect: "write" as const,
      untrusted: false,
    },
    {
      toolId: "erp.transfer",
      server: "erp",
      effect: "financial" as const,
      untrusted: false,
    },
  ];

  it("answers in what the agent can do, not in tool ids", () => {
    // "crm.reply, erp.transfer" is a list. "Alters state, moves money" is the
    // thing somebody approves or refuses.
    const lines = riskSurface(["crm.reply", "erp.transfer"], catalogue).join(
      " ",
    );

    expect(lines).toMatch(/Altera estado em 1/);
    expect(lines).toMatch(/Move dinheiro em 1/);
  });

  it("says an unclassified tool will not run", () => {
    // Otherwise somebody waits for something that never happens.
    const lines = riskSurface(["kb.search"], catalogue).join(" ");
    expect(lines).toMatch(/sem classificação/);
  });

  it("flags what brings outside data in, because that marks the run", () => {
    const lines = riskSurface(["crm.lookup"], catalogue).join(" ");
    expect(lines).toMatch(/dado de fora/);
  });

  it("says plainly when an agent can touch nothing", () => {
    expect(riskSurface([], catalogue).join(" ")).toMatch(
      /só consegue raciocinar/,
    );
  });
});
