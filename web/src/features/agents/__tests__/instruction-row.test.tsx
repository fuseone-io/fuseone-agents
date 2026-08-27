import { useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { InstructionRow } from "@/features/agents/instruction-row";
import type { BlockKind } from "@/features/agents/instruction-blocks";
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

function StatefulRow({
  kind = "objective",
  initial,
}: {
  kind?: BlockKind;
  initial: string;
}) {
  const [text, setText] = useState(initial);
  return (
    <InstructionRow
      block={{ kind, text }}
      at={0}
      tools={{ catalogue: CATALOGUE, policies: [], enabled: ["crm.lookup"] }}
      findings={[]}
      on={{
        change: setText,
        remove: vi.fn(),
        keep: vi.fn(),
        enable: vi.fn(),
        relabel: vi.fn(),
        split: vi.fn(),
        slash: vi.fn(),
        drag: STILL,
      }}
    />
  );
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

  it("keeps focus in the textarea when editing starts", async () => {
    renderRow("Compare os dois lados.");

    await userEvent.click(screen.getByRole("textbox"));

    const editor = screen.getByRole("textbox") as HTMLTextAreaElement;
    expect(editor.tagName).toBe("TEXTAREA");
    expect(editor).toHaveFocus();
  });

  it("renders block titles with enough weight to scan the structure", () => {
    renderRow("Compare os dois lados.");

    expect(screen.getByRole("button", { name: /Objetivo|Purpose/ })).toHaveClass(
      "font-semibold",
      "text-foreground",
    );
  });

  it("opens the enabled tool picker from a how-to-act block", async () => {
    Element.prototype.scrollIntoView = vi.fn();
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <InstructionRow
        block={{ kind: "howToAct", text: "Use " }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [], enabled: ["crm.lookup"] }}
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

    await user.click(screen.getByRole("textbox"));
    await user.type(screen.getByRole("textbox"), "@");

    expect(await screen.findByText("crm.lookup")).toBeInTheDocument();
    expect(screen.queryByText("erp.refund")).not.toBeInTheDocument();

    await user.click(screen.getByText("crm.lookup"));

    expect(onChange).toHaveBeenLastCalledWith("Use crm.lookup");
  });

  it("inserts a cited tool where @ was typed in the middle of a block", async () => {
    Element.prototype.scrollIntoView = vi.fn();
    const onChange = vi.fn();
    render(
      <InstructionRow
        block={{ kind: "howToAct", text: "Use  antes de responder." }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [], enabled: ["crm.lookup"] }}
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

    await userEvent.click(screen.getByRole("textbox"));
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "Use @ antes de responder.", selectionStart: 5 },
    });

    expect(await screen.findByText("crm.lookup")).toBeInTheDocument();

    await userEvent.click(screen.getByText("crm.lookup"));

    expect(onChange).toHaveBeenLastCalledWith(
      "Use crm.lookup antes de responder.",
    );
  });

  it("replaces the typed @ instead of another @ later in the block", async () => {
    Element.prototype.scrollIntoView = vi.fn();
    const onChange = vi.fn();
    render(
      <InstructionRow
        block={{
          kind: "howToAct",
          text: "Use  e avise fulano@acme.com.",
        }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [], enabled: ["crm.lookup"] }}
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

    await userEvent.click(screen.getByRole("textbox"));
    fireEvent.change(screen.getByRole("textbox"), {
      target: {
        value: "Use @ e avise fulano@acme.com.",
        selectionStart: 5,
      },
    });

    expect(await screen.findByText("crm.lookup")).toBeInTheDocument();

    await userEvent.click(screen.getByText("crm.lookup"));

    expect(onChange).toHaveBeenLastCalledWith(
      "Use crm.lookup e avise fulano@acme.com.",
    );
  });

  it("keeps the caret after the tool inserted in the middle of the block", async () => {
    Element.prototype.scrollIntoView = vi.fn();
    render(<StatefulRow initial="Use  para o cliente." />);

    await userEvent.click(screen.getByRole("textbox"));
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "Use @ para o cliente.", selectionStart: 5 },
    });

    expect(await screen.findByText("crm.lookup")).toBeInTheDocument();

    await userEvent.click(screen.getByText("crm.lookup"));

    const editor = screen.getByRole("textbox") as HTMLTextAreaElement;
    expect(editor.value).toBe("Use crm.lookup para o cliente.");
    await waitFor(() =>
      expect(editor.selectionStart).toBe("Use crm.lookup".length),
    );
    expect(editor.selectionEnd).toBe("Use crm.lookup".length);
  });

  it("inserts a cited tool in the middle of any labelled block", async () => {
    Element.prototype.scrollIntoView = vi.fn();
    const onChange = vi.fn();
    render(
      <InstructionRow
        block={{ kind: "objective", text: "Cover  safely." }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [], enabled: ["crm.lookup"] }}
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

    await userEvent.click(screen.getByRole("textbox"));
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "Cover @ safely.", selectionStart: 7 },
    });

    expect(await screen.findByText("crm.lookup")).toBeInTheDocument();
    expect(screen.getByText("erp.refund")).toBeInTheDocument();

    await userEvent.click(screen.getByText("erp.refund"));

    expect(onChange).toHaveBeenLastCalledWith("Cover erp.refund safely.");
  });

  it("keeps the full catalogue picker outside the how-to-act block", async () => {
    Element.prototype.scrollIntoView = vi.fn();
    const user = userEvent.setup();
    render(
      <InstructionRow
        block={{ kind: "objective", text: "Cover " }}
        at={0}
        tools={{ catalogue: CATALOGUE, policies: [], enabled: ["crm.lookup"] }}
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

    await user.click(screen.getByRole("textbox"));
    await user.type(screen.getByRole("textbox"), "@");

    expect(await screen.findByText("crm.lookup")).toBeInTheDocument();
    expect(screen.getByText("erp.refund")).toBeInTheDocument();
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
