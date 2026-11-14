import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TriggerRow } from "@/features/agents/trigger-row";
import { setLocale } from "@/i18n";

/** The event graph at the network boundary: one published, one dangling. */
function stubEvents() {
  const edges = [
    { event: "ticket.handled", from: "triage", to: "notifier" },
    { event: "ticket.handled", from: "triage", to: "auditor" },
    { event: "invoice.paid", to: "billing" },
  ];
  vi.stubGlobal("fetch", async (request: Request) => {
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    const found = path === "/agents/events";
    return new Response(JSON.stringify(found ? { edges } : {}), {
      status: found ? 200 : 404,
      headers: { "Content-Type": "application/json" },
    });
  });
}

function renderEventRow(onChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <TriggerRow
        trigger={{ type: "event", event: "" }}
        onChange={onChange}
        onRemove={() => {}}
      />
    </QueryClientProvider>,
  );
  return onChange;
}

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
  stubEvents();
});
afterEach(() => vi.unstubAllGlobals());

describe("an event trigger's name", () => {
  // A typo publishes and never fires, so the names agents already publish
  // and consume are offered, each once.
  it("offers the known event names", async () => {
    const onChange = renderEventRow();
    await userEvent.click(
      screen.getByRole("combobox", { name: "Suggest known events" }),
    );
    expect(await screen.findAllByText("ticket.handled")).toHaveLength(1);
    await userEvent.click(screen.getByText("invoice.paid"));
    expect(onChange).toHaveBeenCalledWith({ event: "invoice.paid" });
  });

  // An event nothing publishes yet is still a legitimate trigger: one
  // keystroke must arrive, or the field stopped taking a new name.
  it("still takes a name nothing publishes yet", async () => {
    const onChange = renderEventRow();
    await userEvent.type(screen.getByLabelText("Event name"), "x");
    expect(onChange).toHaveBeenCalledWith({ event: "x" });
  });
});
