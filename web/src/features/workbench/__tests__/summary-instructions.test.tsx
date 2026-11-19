import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { setLocale } from "@/i18n";
import { SummaryInstructions } from "@/features/workbench/summary-instructions";

/*
The instructions are read-only on the workbench, never hidden. They change only
through a proposal of the conversation, so this section is the one place the
author reads what the agent is told. A published agent whose draft has not
touched the prose used to show only "the prose has not changed" — the text
itself nowhere on the screen.
*/
describe("the workbench's instructions", () => {
  beforeEach(() => setLocale("pt-BR"));

  const published = "Você triagem chamados de suporte.";

  it("shows the text of a published agent whose draft left it unchanged", () => {
    render(<SummaryInstructions now={published} published={published} />);
    expect(screen.getByText(published)).toBeInTheDocument();
    expect(screen.queryByText(/A prosa não mudou/)).toBeNull();
  });

  it("opens on the difference when the draft changed the prose", () => {
    render(<SummaryInstructions now="Você triagem chamados de vendas." published={published} />);
    expect(screen.getByRole("radio", { name: "Diferença" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("insertion")).toHaveTextContent(/vendas/);
  });

  it("shows the whole new text when the author asks for it", async () => {
    const now = "Você triagem chamados de vendas.";
    render(<SummaryInstructions now={now} published={published} />);
    await userEvent.click(screen.getByRole("radio", { name: "Texto" }));
    expect(screen.getByText(now)).toBeInTheDocument();
  });

  it("offers no difference for an agent never published", () => {
    render(<SummaryInstructions now={published} />);
    expect(screen.getByText(published)).toBeInTheDocument();
    expect(screen.queryByRole("radio")).toBeNull();
  });
});
