import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentFallbackFields } from "@/features/agents/agent-fallback-fields";
import { AgentServingFields } from "@/features/agents/agent-serving-fields";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";

/*
An area's author cannot read the integrations listing, which is the
platform's configuration. The providers they pick from are the reduced
answer, asked for the agent's own scope (NT-019, rule 5).
*/

const PROVIDERS = {
  items: [{ name: "claude-cx", kind: "anthropic", models: ["sonnet"], usable: true }],
};

function stubApi(status = 200) {
  const asked: URL[] = [];
  vi.stubGlobal("fetch", async (input: Request) => {
    const url = new URL(input.url);
    asked.push(url);
    const body = status === 200 ? PROVIDERS : { title: "Not Found", status };
    return new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  });
  return asked;
}

const DRAFT = {
  name: "Atendimento",
  company: "acme",
  area: "cx",
  provider: "",
  model: "",
  instructions: "",
  tools: [],
  budget: { micros: 500_000 },
  triggers: [],
} as unknown as AgentDefinition;

function renderFields() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AgentServingFields draft={DRAFT} patch={vi.fn()} />
      <AgentFallbackFields draft={DRAFT} patch={vi.fn()} />
    </QueryClientProvider>,
  );
}

describe("the provider pickers", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  beforeEach(() => setLocale("en-US"));
  afterEach(() => vi.unstubAllGlobals());

  it("ask /providers for the agent's scope, never the integrations listing", async () => {
    const asked = stubApi();
    renderFields();

    await waitFor(() => expect(asked.length).toBeGreaterThan(0));
    expect(asked.map((url) => url.pathname)).not.toContain("/api/v1/admin/integrations");
    expect(asked.every((url) => url.pathname.endsWith("/providers"))).toBe(true);
    expect(asked.map((url) => url.searchParams.get("scope"))).toContain("acme/cx");
  });

  it("offer the providers the reduced answer names", async () => {
    stubApi();
    renderFields();

    await userEvent.click(screen.getByRole("combobox", { name: "Provider" }));

    expect(await screen.findByRole("option", { name: "claude-cx" })).toBeInTheDocument();
  });

  it("say the area cannot be read when /providers answers 404", async () => {
    stubApi(404);
    renderFields();

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /cannot read what this area uses/,
    );
  });
});
