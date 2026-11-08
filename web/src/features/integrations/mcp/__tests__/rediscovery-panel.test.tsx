import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { RediscoveryPanel } from "@/features/integrations/mcp/rediscovery-panel";
import { setLocale } from "@/i18n";
import { json, stubNetwork } from "@/test/revised-routes";

const at = "2026-11-08T12:00:00Z";
const path = "/admin/integrations/mcp-servers/crm/rediscovery";

type Receipt = { worker: string; generation: number; succeeded: boolean; at: string };

function world(generation: number, receipts: Receipt[], readFails = false) {
  const status = { server: "crm", generation, requestedBy: "usr_bia", requestedAt: at, receipts };
  const answer = (worker: string) => {
    status.receipts = status.receipts.map((r) => (r.worker === worker ? { ...r, generation } : r));
  };
  const requests = stubNetwork([], (method, p) => {
    if (p !== path) return undefined;
    if (method === "POST") return json({ generation: generation + 1 }, 202);
    return readFails ? json({ type: "fuseone:unavailable", status: 503 }, 503) : json(status);
  });
  return { answer, posts: () => requests.filter((r) => r.method === "POST" && r.path === path) };
}

function show(enabled = true) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RediscoveryPanel server="crm" enabled={enabled} />
      <Toaster />
    </QueryClientProvider>,
  );
}

describe("asking every worker to rediscover a tool server", () => {
  beforeEach(() => setLocale("en-US"));
  afterEach(() => vi.unstubAllGlobals());

  it("asks only after the confirmation says what will happen", async () => {
    const user = userEvent.setup();
    const net = world(0, []);
    show();
    await user.click(await screen.findByRole("button", { name: "Rediscover on every worker" }));

    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText(/nothing reconnects/i)).toBeVisible();
    expect(net.posts()).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: "Rediscover" }));
    await waitFor(() => expect(net.posts()).toHaveLength(1));
  });

  it("sends nothing when the confirmation is cancelled", async () => {
    const user = userEvent.setup();
    const net = world(0, []);
    show();
    await user.click(await screen.findByRole("button", { name: "Rediscover on every worker" }));
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel" }));

    expect(net.posts()).toHaveLength(0);
  });

  it("says, worker by worker, who answered the latest request and who did not", async () => {
    world(2, [
      { worker: "worker-a", generation: 2, succeeded: true, at },
      { worker: "worker-b", generation: 1, succeeded: true, at },
      { worker: "worker-c", generation: 2, succeeded: false, at },
    ]);
    show();

    const row = async (worker: string) => (await screen.findByText(worker)).closest("tr")!;
    expect(within(await row("worker-a")).getByText("Answered")).toBeVisible();
    expect(within(await row("worker-b")).getByText("Not answered yet")).toBeVisible();
    expect(within(await row("worker-c")).getByText("Failed; tools unchanged")).toBeVisible();
    expect(screen.getByText(/not the workers that are running/)).toBeVisible();
  });

  it("reads the answers again while a worker is still behind", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const net = world(2, [{ worker: "worker-b", generation: 1, succeeded: true, at }]);
    show();
    expect(await screen.findByText("Not answered yet")).toBeVisible();

    net.answer("worker-b");
    await vi.advanceTimersByTimeAsync(5_000);

    expect(await screen.findByText("Answered")).toBeVisible();
    vi.useRealTimers();
  });

  it("says nobody has asked yet when there is no request", async () => {
    world(0, []);
    show();

    expect(await screen.findByText("Nobody has asked the workers to look again")).toBeVisible();
  });

  it("offers a retry when the answers cannot be read", async () => {
    world(1, [], true);
    show();

    expect(await screen.findByRole("button", { name: "Try again" })).toBeVisible();
  });

  it("cannot be asked of a server that is switched off", async () => {
    world(0, []);
    show(false);

    expect(await screen.findByRole("button", { name: "Rediscover on every worker" })).toBeDisabled();
  });
});
