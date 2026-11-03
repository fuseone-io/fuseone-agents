import { within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { i18next } from "@/i18n";
import {
  agent,
  agentRow,
  cell,
  measures,
  open,
  row,
  serve,
} from "@/features/work-items/__tests__/measures-fixtures";

const AGENTS = [
  agent("reviewer", "acme", "factory", "forbidden"),
  agent("concierge", "acme", "support", "allowed"),
  agent("legacy", "acme", "support"),
];

const standing = (agentId: string, count: number) =>
  row({ agentId }, { standing: count, approved: 3 });

describe.each([
  ["pt-BR", /proíbe aprovações permanentes/i, "Permanentes"],
  ["en-US", /forbids standing approvals/i, "Standing"],
] as const)("standing approvals against the agent's own policy (%s)", (locale, warning, column) => {
  afterEach(async () => {
    vi.unstubAllGlobals();
    await i18next.changeLanguage("pt-BR");
  });

  it("warns when an agent whose policy forbids them has some", async () => {
    await i18next.changeLanguage(locale);
    serve(measures([standing("reviewer", 2)]), AGENTS);
    open();

    const reviewer = await agentRow("reviewer");
    expect(
      await within(reviewer).findByText(warning),
    ).toBeInTheDocument();
  });

  it("only counts them for an agent that allows them, or never said", async () => {
    await i18next.changeLanguage(locale);
    serve(
      measures([standing("concierge", 2), standing("legacy", 1)]),
      AGENTS,
    );
    open();

    const concierge = await agentRow("concierge");
    const legacy = await agentRow("legacy");
    // Wait for the policies to arrive, so an absence is not just early.
    await vi.waitFor(() => expect(cell(concierge, column)).toHaveTextContent(/^2$/));
    expect(cell(legacy, column)).toHaveTextContent(/^1$/);
    expect(within(concierge).queryByText(warning)).toBeNull();
    expect(within(legacy).queryByText(warning)).toBeNull();
  });

  it("says nothing of a forbidding agent with none", async () => {
    await i18next.changeLanguage(locale);
    serve(measures([standing("reviewer", 0)]), AGENTS);
    open();

    const reviewer = await agentRow("reviewer");
    expect(cell(reviewer, column)).toHaveTextContent(/^0$/);
    expect(within(reviewer).queryByText(warning)).toBeNull();
  });
});
