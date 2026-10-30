import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { AgentBudgetSection } from "@/features/agents/agent-budget-section";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";
import { setInstallationCurrency } from "@/lib/format";

function renderSection(micros?: number) {
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
  render(<Harness />);
  return seen;
}

const costField = () => screen.getByLabelText(/Cost per run \(USD\)/);

describe("an agent's per-run cost ceiling", () => {
  beforeEach(() => {
    setLocale("en-US");
    setInstallationCurrency("USD");
  });

  it("is typed as money and stored as micros", async () => {
    const seen = renderSection();
    await userEvent.type(costField(), "500");
    expect(seen.draft?.budget?.micros).toBe(500_000_000);
  });

  it("keeps a decimal comma while it is being typed", async () => {
    const seen = renderSection();
    await userEvent.type(costField(), "0,25");
    expect(costField()).toHaveValue("0,25");
    expect(seen.draft?.budget?.micros).toBe(250_000);
  });

  it("opens an existing ceiling as the amount, not as micros", () => {
    renderSection(1_000_000_000);
    expect(costField()).toHaveValue("1000");
  });
});
