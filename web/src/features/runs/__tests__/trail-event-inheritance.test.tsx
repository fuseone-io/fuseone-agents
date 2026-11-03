import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { TrailEvent } from "@/features/runs/trail-event";
import { i18next as i18n } from "@/i18n";
import type { Step } from "@/lib/api/client";

const REF = "github:fuseone-io/fuseone-agents#204";

const start = (workItem: Record<string, unknown>) =>
  ({
    seq: 1,
    kind: "run_started",
    at: "2026-10-30T12:00:00Z",
    hash: "h",
    payload: { trigger: "event", work_item: workItem },
  }) as Step;

function renderStart(step: Step) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <ul>
          <TrailEvent
            runId="run-1"
            step={step}
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
 * A run's opening line says which card it inherited and from where (#204),
 * and leads to that card's trail.
 */
describe("an inherited work item on run_started", () => {
  it("names the card and the run it came from, in pt-BR", () => {
    renderStart(start({ ref: REF, source: "event", from_run: "run-dev" }));

    const link = screen.getByRole("link", {
      name: `Herdou ${REF} da execução run-dev`,
    });
    expect(link).toHaveAttribute(
      "href",
      "/work-items?ref=github%3Afuseone-io%2Ffuseone-agents%23204",
    );
  });

  it("names a GitHub delivery in en-US", async () => {
    await i18n.changeLanguage("en-US");
    try {
      renderStart(start({ ref: REF, source: "github_delivery" }));
      expect(
        screen.getByRole("link", {
          name: `Inherited ${REF} from a GitHub delivery`,
        }),
      ).toBeInTheDocument();
    } finally {
      await i18n.changeLanguage("pt-BR");
    }
  });

  it("shows nothing for a shape it does not recognise", () => {
    renderStart(start({ ref: REF, source: "claim" }));
    expect(screen.queryByText(/herdou/i)).toBeNull();
  });
});
