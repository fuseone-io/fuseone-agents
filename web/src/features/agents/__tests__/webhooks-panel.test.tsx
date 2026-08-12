import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WebhooksPanel } from "@/features/agents/webhooks-panel";
import type { Webhook } from "@/lib/api/client";

const SECRET = "ejWiCWSIc7lv7W2Jwyf_jXBV_VlDbT2a4q7GsexcJ_0";

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <WebhooksPanel agentId="triage" />
    </QueryClientProvider>,
  );
}

/** Answers the listing with one hook, and the rotation with a secret. */
function stubEndpoints(hook: Webhook) {
  const rotations: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/secret")) {
        rotations.push(url);
        return json({ secret: SECRET, url: "/hooks/crm/ticket" });
      }
      return json({ items: [hook] });
    }),
  );
  return rotations;
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

const closed: Webhook = { path: "crm/ticket", armed: false };
const armed: Webhook = { path: "crm/ticket", armed: true, rotatedBy: "ana" };

describe("the webhooks panel", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("says a declared path cannot fire until somebody generates its key", async () => {
    // The safe state, and a confusing one to discover through silence.
    stubEndpoints(closed);
    renderPanel();

    expect(await screen.findByText(/não dispara/i)).toBeInTheDocument();
  });

  it("shows the key once, with where to send it", async () => {
    stubEndpoints(closed);
    const user = userEvent.setup();
    renderPanel();

    await user.click(
      await screen.findByRole("button", { name: "Gerar chave" }),
    );

    expect(await screen.findByText(SECRET)).toBeInTheDocument();
    // Reading a 43-character secret off a screen is how the wrong one ends up
    // in a configuration file.
    expect(screen.getByRole("button", { name: /copiar/i })).toBeInTheDocument();
    expect(screen.getByText(/POST \/hooks\/crm\/ticket/)).toBeInTheDocument();
  });

  it("warns before replacing a key that already exists", async () => {
    // Rotating breaks every sender configured against the path. Generating
    // the first one breaks nothing, and does not ask.
    stubEndpoints(armed);
    const user = userEvent.setup();
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "Rotacionar" }));

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByText(/passar a receber 401/i)).toBeInTheDocument();
  });

  it("does not rotate until the warning is accepted", async () => {
    const rotations = stubEndpoints(armed);
    const user = userEvent.setup();
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "Rotacionar" }));
    await waitFor(() =>
      expect(screen.getByRole("alertdialog")).toBeInTheDocument(),
    );

    expect(rotations).toHaveLength(0);
  });
});
