import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useMemoryAssertions, type MemoryFilters } from "@/features/memory/api";
import { MemoryBrowserToolbar } from "@/features/memory/memory-browser-toolbar";
import { setLocale } from "@/i18n";

/** The agents and the memory listing at the network; listing queries kept. */
function stubNetwork() {
  const asked: URLSearchParams[] = [];
  const agents = [{ agentId: "security-sentinel", name: "Security Sentinel" }];
  vi.stubGlobal("fetch", async (request: Request) => {
    const url = new URL(request.url);
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (path === "/agents") return Response.json({ items: agents });
    if (path === "/admin/memory/assertions") asked.push(url.searchParams);
    return Response.json({ items: [] });
  });
  return asked;
}

/** The toolbar wired to the listing the way the memory page wires it. */
function Harness({ agentId }: { agentId: string }) {
  const [filters, setFilters] = useState<MemoryFilters>({
    status: "active",
    search: "",
    agentId,
  });
  useMemoryAssertions(filters);
  return (
    <MemoryBrowserToolbar
      filters={filters}
      view="active"
      onFilters={setFilters}
      onView={() => {}}
    />
  );
}

function renderToolbar(agentId = "") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <Harness agentId={agentId} />
    </QueryClientProvider>,
  );
}

const pick = async (name: string) => {
  await userEvent.click(
    screen.getByRole("combobox", { name: "Filter by agent" }),
  );
  await userEvent.click(await screen.findByRole("option", { name }));
};

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
});
afterEach(() => vi.unstubAllGlobals());

describe("the memory agent filter", () => {
  // A case typo in a free id showed nothing; the agents are chosen instead,
  // and the chosen id is what the listing is asked for.
  it("sends the chosen agent's id", async () => {
    const asked = stubNetwork();
    renderToolbar();
    await pick("Security Sentinel");
    await waitFor(() =>
      expect(asked.at(-1)?.get("agentId")).toBe("security-sentinel"),
    );
  });

  // "All agents" is the absence of a filter, not an id nobody has.
  it("sends no agent for all agents", async () => {
    const asked = stubNetwork();
    renderToolbar("security-sentinel");
    await waitFor(() => expect(asked).toHaveLength(1));
    await pick("All agents");
    await waitFor(() => expect(asked).toHaveLength(2));
    expect(asked[0]?.get("agentId")).toBe("security-sentinel");
    expect(asked[1]?.has("agentId")).toBe(false);
  });
});
