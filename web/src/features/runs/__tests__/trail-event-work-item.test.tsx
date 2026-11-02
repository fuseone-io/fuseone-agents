import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { TrailEvent } from "@/features/runs/trail-event";
import type { Step } from "@/lib/api/client";

const claim = {
  seq: 5,
  kind: "work_item_claimed",
  at: "2026-10-30T12:00:00Z",
  hash: "h",
  payload: { ref: "github:fuseone-io/fuseone-agents#123" },
} as Step;

function renderClaim() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <ul>
          <TrailEvent
            runId="run-1"
            step={claim}
            live={false}
            last
            showHashes={false}
          />
        </ul>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/*
 * A claim in the run trail names its card, and the name leads to the card's
 * own trail: every run that worked on it, not only this one.
 */
describe("a claim in the run trail", () => {
  it("links the claimed reference to its work item trail", () => {
    renderClaim();

    const link = screen.getByRole("link", {
      name: "Item: github:fuseone-io/fuseone-agents#123",
    });
    expect(link).toHaveAttribute(
      "href",
      "/work-items?ref=github%3Afuseone-io%2Ffuseone-agents%23123",
    );
  });
});
