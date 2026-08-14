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
    name: "Sem estorno automático",
    resource: "erp.refund",
    effect: "deny",
    // Enforcing rather than observing: a rule somebody is only watching
    // does not refuse anything, so the prose is not promising the impossible.
    mode: "enforce",
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
      findings={[]}
      onKeep={vi.fn()}
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

describe("a sentence the policy refuses", () => {
  it("is answered in the block that said it, with two real exits", async () => {
    // Not a banner on the card: a warning at the top leaves somebody hunting
    // for the sentence, and by the third visit they stop reading it.
    const onChange = vi.fn();
    render(
      <InstructionRow
        block={{
          kind: "howToAct",
          text: "Compare os dois lados. Se precisar, use erp.refund.",
        }}
        onChange={onChange}
        onRemove={vi.fn()}
        tools={{ catalogue: CATALOGUE, policies: DENIES }}
        findings={[{ at: 0, tool: "erp.refund", because: "POL-114" }]}
        onKeep={vi.fn()}
      />,
    );

    expect(screen.getByText("POL-114")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Remover a frase/ }));

    // The sentence, and not the block: an author told about one sentence
    // must not lose the three around it.
    expect(onChange).toHaveBeenCalledWith("Compare os dois lados.");
  });
});
