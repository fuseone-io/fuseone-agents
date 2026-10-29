import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentServingFields } from "@/features/agents/agent-serving-fields";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";

function stubIntegrations() {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ providers: [], presets: [] }), {
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
      <AgentServingFields
        draft={
          {
            name: "Sentinela",
            company: "default",
            area: "platform",
            provider: "openai",
            model: "devstack",
            instructions: "",
            tools: [],
            budget: { micros: 500_000 },
            triggers: [],
            ...draft,
          } as unknown as AgentDefinition
        }
        patch={patch}
      />
    </QueryClientProvider>,
  );
  return patch;
}

/*
 * The draft and the endpoint are each proven separately; this is the missing
 * middle. Without it, removing the select — or wiring it to the wrong field —
 * keeps every other test green while the Studio quietly loses the one control
 * that stops an agent reporting in the wrong language (#156).
 */
describe("the reply-language select", () => {
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

  it("patches the draft with the chosen language", async () => {
    const user = userEvent.setup();
    const patch = renderFields({});

    await user.click(screen.getByRole("combobox", { name: "Reply language" }));
    await user.click(await screen.findByRole("option", { name: "pt-BR" }));

    expect(patch).toHaveBeenCalledWith({ replyLocale: "pt-BR" });
  });

  it("returns the field to empty when the pin is removed", async () => {
    const user = userEvent.setup();
    const patch = renderFields({ replyLocale: "pt-BR" });

    await user.click(screen.getByRole("combobox", { name: "Reply language" }));
    await user.click(
      await screen.findByRole("option", { name: "No pin — the model chooses" }),
    );

    expect(patch).toHaveBeenCalledWith({ replyLocale: "" });
  });
});

/*
 * A regression guard on the class, stated as what it is. The defect is
 * layout: the primitive's trigger is w-fit, so a long selected label — the
 * one-hour cache line — grows the trigger past its grid track and over the
 * neighbouring select, and the primitive's own line-clamp only truncates a
 * trigger whose width is bounded. jsdom lays nothing out, so the overflow
 * itself cannot be measured here; the class that bounds the trigger to its
 * track is the mechanism, and its absence is the bug.
 */
describe("the serving selects", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setLocale("en-US");
    stubIntegrations();
  });

  it("every trigger fills its grid track instead of sizing to its label", () => {
    renderFields({ cachePrefixTtl: "1h" });

    for (const trigger of screen.getAllByRole("combobox")) {
      expect(trigger.className).toContain("w-full");
    }
  });
});
