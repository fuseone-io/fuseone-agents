import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DecisionPanel } from "@/features/approvals/decision-panel";
import type { PendingApproval } from "@/lib/api/client";

afterEach(() => vi.unstubAllGlobals());

describe("approval evidence", () => {
  it("shows the inspected subscription before enabling approval", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({
      kind: "gravitee_subscription",
      gravitee: {
        subscriptionId: "sub-42",
        status: "PENDING",
        application: { id: "app-1", name: "Portal", primaryOwnerEmail: "dev@example.com" },
        api: { id: "checkout-api", name: "Checkout" },
        plan: { id: "plan-1", name: "API keys" },
        planSecurity: "API_KEY",
        requestedExpiration: "2026-09-13T12:00:00Z",
        remoteUpdatedAt: "2026-09-11T12:00:00Z",
      },
    })));

    renderPanel();

    expect(await screen.findByText("Subscription inspecionada")).toBeVisible();
    expect(screen.getByText("Portal")).toBeVisible();
    expect(screen.getByText("dev@example.com")).toBeVisible();
    expect(screen.getByText("Checkout")).toBeVisible();
    expect(screen.getByRole("button", { name: "Aprovar" })).toBeEnabled();
  });

  it("keeps rejection available when the snapshot cannot be verified", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(
      { title: "Conflict", status: 409, detail: "evidence unavailable" },
      { status: 409, headers: { "Content-Type": "application/problem+json" } },
    )));

    renderPanel();

    expect(await screen.findByText("A evidência inspecionada está indisponível")).toBeVisible();
    expect(screen.getByRole("button", { name: "Aprovar" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Negar" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeVisible();
  });
});

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <DecisionPanel item={approval()} onDecided={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function approval(): PendingApproval {
  return {
    runId: "run-gravitee",
    agentId: "support",
    tool: "gravitee.apim.accept_subscription",
    effect: "write",
    rule: "approval",
    atSeq: 7,
    requestedAt: "2026-09-11T12:00:00Z",
    scope: { company: "acme", area: "platform" },
  };
}
