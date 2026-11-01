import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WebhooksPanel } from "@/features/agents/webhooks-panel";
import { setLocale } from "@/i18n";
import type { Webhook } from "@/lib/api/client";

const hook: Webhook = { path: "gh/reviewer", armed: false, from: "github" };

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function renderPanel() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      return url.includes("/secret")
        ? json({ secret: "k3y", url: "/hooks/gh/reviewer" })
        : json({ items: [hook] });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <WebhooksPanel agentId="reviewer" />
    </QueryClientProvider>,
  );
}

/*
A GitHub webhook's key goes into GitHub's Secret field, not into a header.

GitHub signs every delivery with it and never sends it, so telling the
operator to send it in X-FuseOne-Secret, or to send an Idempotency-Key GitHub
cannot send, would be configuring a sender that does not exist.
*/
describe("the webhooks panel, for a webhook GitHub sends", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setLocale("en-US");
  });
  afterEach(() => vi.unstubAllGlobals());

  it("says the path comes from GitHub", async () => {
    renderPanel();
    expect(await screen.findByText("GitHub")).toBeInTheDocument();
  });

  it("tells where in GitHub the key goes", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.click(await screen.findByRole("button", { name: /generate/i }));

    expect(
      await screen.findByText("The GitHub webhook's Secret field"),
    ).toBeInTheDocument();
    expect(screen.getByText(/application\/json/)).toBeInTheDocument();
    expect(screen.queryByText(/X-FuseOne-Secret/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Idempotency-Key/)).not.toBeInTheDocument();
  });
});
