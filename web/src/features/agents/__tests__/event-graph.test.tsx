import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { EventGraph } from "@/features/agents/event-graph";
import { setLocale } from "@/i18n";

afterEach(() => vi.unstubAllGlobals());

describe("the event graph", () => {
  // The dangling edge is the point: an event nobody publishes is drawn
  // with the reason, not hidden.
  it("draws each edge from the graph the server answers", async () => {
    setLocale("en-US");
    const edges = [{ event: "invoice.paid", to: "billing" }];
    vi.stubGlobal("fetch", async () =>
      Response.json({ edges }, { status: 200 }),
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <EventGraph />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("invoice.paid")).toBeInTheDocument();
    expect(screen.getByText("billing")).toBeInTheDocument();
    expect(screen.getByText("nobody publishes")).toBeInTheDocument();
  });
});
