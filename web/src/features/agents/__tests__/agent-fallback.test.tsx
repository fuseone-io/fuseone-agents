import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentFallbackFields } from "@/features/agents/agent-fallback-fields";
import { changesBetween, useAgentDraft } from "@/features/agents/agent-draft";
import { setLocale } from "@/i18n";
import type { AgentDefinition, AgentDetail } from "@/lib/api/client";

const withFallback = {
  agent: {
    agentId: "cobranca",
    name: "Cobrança amigável",
    scope: { company: "default", area: "financeiro" },
    provider: "anthropic",
    model: "claude-opus-5",
    fallback: { provider: "openai", model: "gpt-test" },
    tools: ["crm.lookup"],
    budget: { micros: 500_000, steps: 60 },
  },
  instructions: "Cobre com educação.",
} as unknown as AgentDetail;

const base = {
  name: "Sentinela",
  company: "default",
  area: "platform",
  provider: "anthropic",
  model: "claude-opus-5",
  instructions: "",
  tools: [],
  budget: { micros: 500_000 },
  triggers: [],
} as unknown as AgentDefinition;

function stubIntegrations() {
  const providers = [
    { name: "anthropic", kind: "anthropic", baseUrl: "", enabled: true, hasKey: true },
    { name: "openai", kind: "openai", baseUrl: "", enabled: true, hasKey: true },
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ providers, presets: [], mcpServers: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
}

function renderFields(draft: Partial<AgentDefinition>, patch = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AgentFallbackFields draft={{ ...base, ...draft }} patch={patch} />
    </QueryClientProvider>,
  );
  return patch;
}

/*
 * Publishing writes the draft whole: a fallback the draft drops is a fallback
 * deleted on the next edit, and nobody finds out until the outage it was for.
 */
describe("the fallback pair in the draft", () => {
  it("survives an edit made for another reason", () => {
    const { result } = renderHook(() => useAgentDraft(withFallback));
    act(() => result.current.patch({ instructions: "Cobre com calma." }));
    expect(result.current.draft.fallback).toEqual({ provider: "openai", model: "gpt-test" });
  });

  it("is named among the changes publishing will make", () => {
    const after = { ...base, fallback: { provider: "openai", model: "gpt-test" } };
    const fields = changesBetween(base, after).map((c) => c.field);
    expect(fields).toContain("agents.fieldFallback");
  });
});

describe("the fallback provider select", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  });

  beforeEach(() => {
    vi.restoreAllMocks();
    setLocale("en-US");
    stubIntegrations();
  });

  it("starts a fallback on the chosen provider, with its model still to pick", async () => {
    const user = userEvent.setup();
    const patch = renderFields({});

    await user.click(screen.getByRole("combobox", { name: "Fallback provider" }));
    await user.click(await screen.findByRole("option", { name: "openai" }));

    expect(patch).toHaveBeenCalledWith({ fallback: { provider: "openai", model: "" } });
  });

  it("removes the fallback when none is chosen", async () => {
    const user = userEvent.setup();
    const patch = renderFields({ fallback: { provider: "openai", model: "gpt-test" } });

    await user.click(screen.getByRole("combobox", { name: "Fallback provider" }));
    await user.click(await screen.findByRole("option", { name: "No fallback" }));

    expect(patch).toHaveBeenCalledWith({ fallback: undefined });
  });

  it("says an autonomous run will not switch", () => {
    renderFields({ fallback: { provider: "openai", model: "gpt-test" } });
    expect(screen.getByText(/autonomous/i)).toBeInTheDocument();
  });
});
