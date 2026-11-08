import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { ProviderEditor } from "@/features/integrations/provider-editor";
import { ProviderCard } from "@/features/integrations/provider-card";
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
    path === "/admin/integrations" ? json({ mcpServers: [], providers: providers.state.items, presets: [] }) : undefined,
  );
  return { providers, writes: (method: string) => requests.filter((r) => r.method === method) };
}

function show(node: ReactNode) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {node}
      <Toaster />
    </QueryClientProvider>,
  );
}

describe("a model provider written over what was read", () => {
  beforeEach(() => {
    setLocale("en-US");
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  it("is read by name when its editor opens, and saved under that revision", async () => {
    const net = world();
    show(<ProviderEditor provider={gateway} onClose={vi.fn()} />);
    await userEvent.setup().click(await screen.findByRole("button", { name: "Save" }));

    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-gateway-1"');
  });

  it("keeps the edit when somebody wrote first, and reloads only when asked", async () => {
    const user = userEvent.setup();
    const net = world();
    show(<ProviderEditor provider={gateway} onClose={vi.fn()} />);
    const address = await screen.findByLabelText("Address");
    await user.clear(address);
    await user.type(address, "https://llm.example.com/mine");
    net.providers.elsewhere("gateway", { baseUrl: "https://llm.example.com/theirs" });
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Somebody changed this while you were editing")).toBeVisible();
    expect(screen.getByLabelText("Address")).toHaveValue("https://llm.example.com/mine");
    await user.click(screen.getByRole("button", { name: "Reload what is stored" }));
    await waitFor(() => expect(screen.getByLabelText("Address")).toHaveValue("https://llm.example.com/theirs"));
    // Reloading throws the draft away; it never sends it.
    expect(net.writes("PUT")).toHaveLength(1);
  });

  it("is removed under the revision read when its confirmation opened", async () => {
    const user = userEvent.setup();
    const net = world();
    show(<ProviderCard provider={gateway} onEdit={vi.fn()} />);
    net.providers.elsewhere("gateway", {});
    await user.click(screen.getByRole("button", { name: "Remove gateway?" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Remove" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(net.writes("DELETE")).toHaveLength(1));
    expect(net.writes("DELETE")[0]?.ifMatch).toBe('"v1-gateway-2"');
  });

  it("offers no form over a read that came back without a revision", async () => {
    const net = world();
    net.providers.reads.mode = "no-etag";
    show(<ProviderEditor provider={gateway} onClose={vi.fn()} />);

    expect(await screen.findByRole("button", { name: "Try again" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });
});
