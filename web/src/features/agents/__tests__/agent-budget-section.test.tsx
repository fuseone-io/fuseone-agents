import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentBudgetSection } from "@/features/agents/agent-budget-section";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";
import { setInstallationCurrency } from "@/lib/format";

function renderSection(
  micros?: number,
  money: Promise<string> = Promise.resolve("USD"),
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ currency: await money }), {
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  const seen: { draft?: AgentDefinition } = {};
  function Harness() {
    const [draft, setDraft] = useState({
      budget: { micros },
    } as AgentDefinition);
    seen.draft = draft;
    return (
      <AgentBudgetSection
        draft={draft}
        patch={(over) => setDraft((d) => ({ ...d, ...over }))}
      />
    );
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>,
  );
  return seen;
}

const costField = () => screen.findByLabelText(/Cost per run \(USD\)/);

describe("an agent's per-run cost ceiling", () => {
  beforeEach(() => {
    setLocale("en-US");
    setInstallationCurrency("BRL");
  });

  it("is typed as money and stored as micros", async () => {
    const seen = renderSection();
    await userEvent.type(await costField(), "500");
    expect(seen.draft?.budget?.micros).toBe(500_000_000);
  });

  it("keeps a decimal comma while it is being typed", async () => {
    const seen = renderSection();
    await userEvent.type(await costField(), "0,25");
    expect(await costField()).toHaveValue("0,25");
    expect(seen.draft?.budget?.micros).toBe(250_000);
  });

  it("opens an existing ceiling as the amount, not as micros", async () => {
    renderSection(1_000_000_000);
    expect(await costField()).toHaveValue("1000");
  });

  it("never shows text that disagrees with the ceiling it will save", async () => {
    const seen = renderSection(5_000_000);
    const field = await costField();
    await userEvent.tripleClick(field);
    await userEvent.keyboard("abc");
    expect(field).toHaveValue("5");
    expect(seen.draft?.budget?.micros).toBe(5_000_000);
  });

  it("names the installation's currency once it is known, not the default", async () => {
    let answer: (currency: string) => void = () => {};
    renderSection(undefined, new Promise((resolve) => (answer = resolve)));
    expect(
      await screen.findByLabelText(/Cost per run \(BRL\)/),
    ).toBeInTheDocument();
    answer("USD");
    expect(await costField()).toBeInTheDocument();
  });
});
