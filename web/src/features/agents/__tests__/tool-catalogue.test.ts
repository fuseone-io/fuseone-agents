import { describe, expect, it } from "vitest";
import { grouped, matching } from "@/features/agents/tool-catalogue";
import type { Tool } from "@/lib/api/client";

function tool(toolId: string, over: Partial<Tool> = {}): Tool {
  return {
    toolId,
    server: toolId.split(".")[0] ?? "",
    effect: "read",
    untrusted: false,
    ...over,
  } as Tool;
}

describe("the catalogue an author picks from", () => {
  it("groups by the server a tool came from", () => {
    // A tool is named after its server, so the list is already sorted into
    // groups; refusing to draw them makes the reader do it.
    const groups = grouped(
      [tool("erp.transfer"), tool("crm.reply"), tool("erp.balance")],
      [],
    );

    expect(groups.map((g) => g.server)).toEqual(["crm", "erp"]);
    expect(groups[1]?.tools.map((t) => t.toolId)).toEqual([
      "erp.balance",
      "erp.transfer",
    ]);
  });

  it("counts what this agent may already reach in each server", () => {
    // The question somebody actually has is "what can it touch in the ERP",
    // and it has to be answerable without opening the group.
    const groups = grouped(
      [tool("erp.transfer"), tool("erp.balance"), tool("crm.reply")],
      ["erp.balance"],
    );

    expect(groups.find((g) => g.server === "erp")?.granted).toBe(1);
    expect(groups.find((g) => g.server === "crm")?.granted).toBe(0);
  });

  it("searches the description as well as the identifier", () => {
    // An author looking for a refund knows what they want to do, not what
    // somebody called it.
    const catalogue = [
      tool("erp.refund", { description: "Reverse a transfer already made" }),
      tool("crm.reply", { description: "Send a reply to the customer" }),
    ];

    expect(matching(catalogue, "reverse").map((t) => t.toolId)).toEqual([
      "erp.refund",
    ]);
    expect(matching(catalogue, "erp.").map((t) => t.toolId)).toEqual([
      "erp.refund",
    ]);
  });

  it("answers with everything when nothing was typed", () => {
    const catalogue = [tool("crm.reply")];
    expect(matching(catalogue, "  ")).toEqual(catalogue);
  });
});
