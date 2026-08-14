import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InstructionRow } from "@/features/agents/instruction-row";
import type { Policy, Tool } from "@/lib/api/client";

/*
Writing prose, and seeing what the platform understood from it.

The chips are a rendering: what the payload carries is the identifier
somebody could have typed. Which is why the row swaps between a textarea and
read prose rather than editing chips in place — the browser already owns the
caret, the paste and the undo stack.
*/

const CATALOGUE: Tool[] = [
  { toolId: "crm.lookup", server: "crm", effect: "read", untrusted: true },
  { toolId: "erp.refund", server: "erp", effect: "financial", untrusted: true },
];

const DENIES = [
  {
    code: "POL-114",
    scope: { company: "acme", area: "cx" },
    tools: ["erp.refund"],
    verdict: "block",
    enabled: true,
  },
] as unknown as Policy[];

function renderRow(text: string, policies: Policy[] = [], onChange = vi.fn()) {
  render(
    <InstructionRow
      block={{ kind: "objective", text }}
      onChange={onChange}
      onRemove={vi.fn()}
      tools={{ catalogue: CATALOGUE, policies }}
    />,
  );
  return onChange;
}

describe("a block of an instruction", () => {
  it("shows the tools the sentence names", () => {
    renderRow("Use crm.lookup para achar o cliente.");

    expect(screen.getByText("crm.lookup")).toBeInTheDocument();
  });

  it("marks a tool the policy in force denies", async () => {
    // The sentence promises what the platform will refuse, and the person who
    // finds out otherwise is whoever reads the run afterwards.
    renderRow("Se precisar, use erp.refund.", DENIES);

    const chip = screen.getByText("erp.refund");
    expect(chip.className).toMatch(/danger/);
  });

  it("becomes a plain textarea to type in", async () => {
    // Editing chips in place means owning the caret and the paste, which the
    // browser already does correctly.
    const onChange = renderRow("Compare os dois lados.");

    await userEvent.click(screen.getByRole("textbox"));
    await userEvent.type(screen.getByRole("textbox"), "!");

    expect(onChange).toHaveBeenCalledWith("Compare os dois lados.!");
  });
});
