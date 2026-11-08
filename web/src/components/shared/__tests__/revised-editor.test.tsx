import { focusManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ProviderEditor } from "@/features/integrations/provider-editor";
import type { ModelProvider } from "@/features/integrations/api";
import { setLocale } from "@/i18n";
import { json, revisedCollection, stubNetwork } from "@/test/revised-routes";

const gateway: ModelProvider = {
  name: "gateway", kind: "openai_compatible", baseUrl: "https://llm.example.com/a",
  models: ["m1"], enabled: true, hasKey: true,
};

function world() {
  const providers = revisedCollection("/admin/integrations/providers", [gateway]);
  const requests = stubNetwork([providers], (_method, path) =>
    path === "/admin/integrations" ? json({ mcpServers: [], providers: [], presets: [] }) : undefined,
  );
  const reads = () =>
    requests.filter((r) => r.method === "GET" && r.path === "/admin/integrations/providers/gateway").length;
  return { providers, reads };
}

const client = () => new QueryClient({ defaultOptions: { queries: { retry: false } } });
const editor = (c: QueryClient) => (
  <QueryClientProvider client={c}>
    <ProviderEditor provider={gateway} onClose={vi.fn()} />
  </QueryClientProvider>
);

describe("an editor seeded from a read", () => {
  beforeEach(() => {
    setLocale("en-US");
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  // Opened before, its copy is cached. Seeding from it would show a version
  // that changed since, and the read arriving after would swap the form.
  it("is not seeded from a copy cached by an earlier opening", async () => {
    const net = world();
    const c = client();
    const first = render(editor(c));
    await screen.findByLabelText("Address");
    first.unmount();
    net.providers.elsewhere("gateway", { baseUrl: "https://llm.example.com/theirs" });
    render(editor(c));

    expect(await screen.findByLabelText("Address")).toHaveValue("https://llm.example.com/theirs");
  });

  // Somebody else's write must not replace what is being typed: only a save
  // refused as stale, or a reload asked for, does. So focus reads nothing.
  it("reads nothing when the window regains focus", async () => {
    const user = userEvent.setup();
    const net = world();
    render(editor(client()));
    const address = await screen.findByLabelText("Address");
    await user.type(address, "/mine");
    const before = net.reads();
    net.providers.elsewhere("gateway", { baseUrl: "https://llm.example.com/theirs" });
    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      await Promise.resolve();
    });

    expect(net.reads()).toBe(before);
    expect(screen.getByLabelText("Address")).toHaveValue("https://llm.example.com/a/mine");
    focusManager.setFocused(undefined);
  });
});
