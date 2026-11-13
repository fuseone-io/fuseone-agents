import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { ExperimentStart } from "@/features/experiments/experiment-start";
import { renderWithClient, stubApi } from "@/features/experiments/__tests__/experiment-fixtures";
import { formatMicros } from "@/lib/format";

/*
An older version is configured and priced as itself. The form offers the models of the
chosen version's provider and states the cost at that version's ceiling: the run that opens
is that version, and the current one's numbers would understate what it may spend.
*/

const base = { scope: { company: "acme", area: "cx" }, name: "Triage", tools: [], publishedAt: "2026-11-01T10:00:00Z" };
const current = { ...base, agentId: "triage", versionId: "v2bbbbbbbbbb", provider: "anthropic", model: "claude-x", budget: { micros: 500_000, steps: 20 }, latest: true };
const older = { ...current, versionId: "v1aaaaaaaaaa", provider: "openai", model: "gpt-y", budget: { micros: 2_000_000, steps: 20 }, latest: false };
const versions = [current, older].map((v) => ({ versionId: v.versionId, publishedAt: v.publishedAt, latest: v.latest }));

function showStart() {
  stubApi({
    "GET /agents/triage": { agent: current, versions },
    "GET /agents/triage?version=v1aaaaaaaaaa": { agent: older, versions },
    "GET /agents/triage?version=v2bbbbbbbbbb": { agent: current, versions },
    "GET /agents/triage/regressions": { items: [{ id: "a" }, { id: "b" }, { id: "c" }] },
    // Asked for the agent's own scope, so an area's author is answered (NT-019).
    "GET /providers?scope=acme%2Fcx": {
      items: [
        { name: "anthropic", kind: "anthropic", models: ["claude-x"], usable: true },
        { name: "openai", kind: "openai_compatible", models: ["gpt-y"], usable: true },
      ],
    },
  });
  renderWithClient(<ExperimentStart agentId="triage" onClose={vi.fn()} onStarted={vi.fn()} />);
}

async function chooseOlder(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText(new RegExp(`até ${escape(formatMicros(500_000))} cada`));
  await user.click(screen.getByRole("combobox", { name: "Versão do agente" }));
  await user.click(await screen.findByRole("option", { name: "v1aaaaaaa" }));
}

// Escaped for a pattern, with the currency's non-breaking space read as the plain one the
// matcher normalises it to.
function escape(text: string) {
  return text.replace(/\s/g, " ").replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

describe("starting an experiment on an older version", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  it("states the cost at that version's ceiling", async () => {
    showStart();
    const user = userEvent.setup();

    await chooseOlder(user);

    expect(
      await screen.findByText(new RegExp(`até ${escape(formatMicros(2_000_000))} cada: até ${escape(formatMicros(6_000_000))}`)),
    ).toBeInTheDocument();
  });

  it("offers the models of that version's provider", async () => {
    showStart();
    const user = userEvent.setup();

    await chooseOlder(user);
    await user.click(screen.getByRole("combobox", { name: "Modelo" }));

    expect(await screen.findByRole("option", { name: "gpt-y" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "claude-x" })).not.toBeInTheDocument();
  });
});
