import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { ChannelEditor } from "@/features/channels/channel-editor";
import type { components } from "@/lib/api/schema.gen";
import { setLocale } from "@/i18n";
import { revisedCollection, stubNetwork } from "@/test/revised-routes";

type Channel = components["schemas"]["Channel"];
const slack: Channel = {
  name: "acme", kind: "slack", workspace: "acme", deliveryMode: "http",
  enabled: true, hasCredential: true, conversations: [],
};

function world() {
  const channels = revisedCollection("/admin/channels", [slack]);
  const requests = stubNetwork([channels], () => undefined);
  return { channels, writes: (method: string) => requests.filter((r) => r.method === method) };
}

function show() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ChannelEditor channel={slack} kinds={["slack"]} onClose={vi.fn()} />
      <Toaster />
    </QueryClientProvider>,
  );
}

describe("a channel connection written over what was read", () => {
  beforeEach(() => {
    setLocale("en-US");
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
  });
  afterEach(() => vi.unstubAllGlobals());

  it("is read by name when its editor opens, and saved under that revision", async () => {
    const net = world();
    show();
    await userEvent.setup().click(await screen.findByRole("button", { name: "Save" }));

    await waitFor(() => expect(net.writes("PUT")).toHaveLength(1));
    expect(net.writes("PUT")[0]?.ifMatch).toBe('"v1-acme-1"');
  });

  // Any write since moves the connection's revision, including a conversation
  // attached to it, so the save is refused rather than landing over an answer
  // that no longer describes it.
  it("keeps the edit when something changed since, and reloads only when asked", async () => {
    const user = userEvent.setup();
    const net = world();
    show();
    const workspace = await screen.findByLabelText("Workspace");
    await user.clear(workspace);
    await user.type(workspace, "mine");
    net.channels.elsewhere("acme", { workspace: "theirs" });
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Somebody changed this while you were editing")).toBeVisible();
    expect(screen.getByLabelText("Workspace")).toHaveValue("mine");
    await user.click(screen.getByRole("button", { name: "Reload what is stored" }));
    await waitFor(() => expect(screen.getByLabelText("Workspace")).toHaveValue("theirs"));
  });
});
