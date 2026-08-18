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

/** Nobody is dragging in these: the reordering has its own test. */
const STILL = { onStart: () => {}, onOver: () => {}, onDrop: () => {} };

function renderRow(text: string, policies: Policy[] = [], onChange = vi.fn()) {
  render(
    <InstructionRow
      block={{ kind: "objective", text }}
      at={0}
      tools={{ catalogue: CATALOGUE, policies }}
      findings={[]}
      on={{
        change: onChange,
        remove: vi.fn(),
        keep: vi.fn(),
        enable: vi.fn(),
        relabel: vi.fn(),
        split: vi.fn(),
        slash: vi.fn(),
        drag: STILL,
      }}
    />,
  );
  return onChange;
}

describe("a block of an instruction", () => {
  it("keeps long prose inside the editable row", () => {
    const longWord = "github_issue_comment_body_".repeat(12);
    const { container } = render(
      <InstructionRow
        block={{ kind: "objective", text: longWord }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [] }}
        findings={[]}
        on={{
          change: vi.fn(),
          remove: vi.fn(),
          keep: vi.fn(),
          enable: vi.fn(),
          relabel: vi.fn(),
          split: vi.fn(),
          slash: vi.fn(),
          drag: STILL,
        }}
      />,
    );

    expect(container.firstElementChild).toHaveClass("min-w-0");
    expect(screen.getByText(longWord).closest("p")).toHaveClass(
      "min-w-0",
      "break-words",
    );
  });

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
        at={0}
        tools={{ catalogue: CATALOGUE, policies: DENIES }}
        findings={[
          { at: 0, tool: "erp.refund", why: "refused", because: "POL-114" },
        ]}
        on={{
          change: onChange,
          remove: vi.fn(),
          keep: vi.fn(),
          enable: vi.fn(),
          relabel: vi.fn(),
          split: vi.fn(),
          slash: vi.fn(),
          drag: STILL,
        }}
      />,
    );

    expect(screen.getByText("POL-114")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Remover a frase/ }));

    // The sentence, and not the block: an author told about one sentence
    // must not lose the three around it.
    expect(onChange).toHaveBeenCalledWith("Compare os dois lados.");
  });
});

/*
A tool the text names and the agent does not hold.

The same warning, a different fix. Nothing about the sentence is wrong: what
is missing is a checkbox, so the first exit is to tick it — and "keep it, it
explains" is not offered, because a sentence naming a tool nobody granted is
not explaining a rule to anybody.
*/
describe("a tool cited and not enabled", () => {
  it("offers to enable it, and enables that tool", async () => {
    const onEnable = vi.fn();
    render(
      <InstructionRow
        block={{ kind: "howToAct", text: "Se precisar, use crm.lookup." }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [] }}
        findings={[{ at: 0, tool: "crm.lookup", why: "notEnabled" }]}
        on={{
          change: vi.fn(),
          remove: vi.fn(),
          keep: vi.fn(),
          enable: onEnable,
          relabel: vi.fn(),
          split: vi.fn(),
          slash: vi.fn(),
          drag: STILL,
        }}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /Habilitar a ferramenta/ }),
    );

    expect(onEnable).toHaveBeenCalledWith("crm.lookup");
  });

  it("does not offer to keep a sentence that explains nothing", () => {
    render(
      <InstructionRow
        block={{ kind: "howToAct", text: "Se precisar, use crm.lookup." }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [] }}
        findings={[{ at: 0, tool: "crm.lookup", why: "notEnabled" }]}
        on={{
          change: vi.fn(),
          remove: vi.fn(),
          keep: vi.fn(),
          enable: vi.fn(),
          relabel: vi.fn(),
          split: vi.fn(),
          slash: vi.fn(),
          drag: STILL,
        }}
      />,
    );

    expect(
      screen.queryByRole("button", { name: /Manter, é explicação/ }),
    ).not.toBeInTheDocument();
  });
});
