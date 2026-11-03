import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useActiveScope } from "@/features/scope/active-scope";
import { useWorkItemMeasures } from "@/features/work-items/api";
import {
  agent,
  agentRow,
  measureCalls,
  measures,
  open,
  row,
  serve,
  stubSelectDom,
} from "@/features/work-items/__tests__/measures-fixtures";

const AGENTS = [
  agent("developer", "acme", "factory"),
  agent("analyst", "acme", "finance"),
  agent("auditor", "globex", "security"),
];

const choose = (company: string, area = "") =>
  act(() => useActiveScope.getState().choose({ company, area }));

async function pick(control: RegExp, option: string) {
  await userEvent.click(screen.getByRole("combobox", { name: control }));
  await userEvent.click(await screen.findByRole("option", { name: option }));
}

const last = (asked: URL[]) => measureCalls(asked).at(-1)?.searchParams;

describe("the weekly panel's filters, inside the active scope", () => {
  beforeEach(stubSelectDom);
  afterEach(() => {
    vi.unstubAllGlobals();
    useActiveScope.getState().choose({ company: "", area: "" });
  });

  it("sends the area the reader picks", async () => {
    const asked = serve(measures([row()]), AGENTS);
    choose("acme");
    open();
    await agentRow("developer");

    await pick(/filtrar por área/i, "factory");

    await vi.waitFor(() => expect(last(asked)?.get("area")).toBe("factory"));
    expect(last(asked)?.get("company")).toBe("acme");
  });

  it("sends the role the reader picks", async () => {
    const asked = serve(measures([row()]), AGENTS);
    choose("acme");
    open();
    await agentRow("developer");

    await pick(/filtrar por papel/i, "developer");

    await vi.waitFor(() =>
      expect(last(asked)?.get("agentId")).toBe("developer"),
    );
  });

  it("clears both filters when the scope changes, and asks in the new scope only", async () => {
    const asked = serve(measures([row()]), AGENTS);
    choose("acme");
    open();
    await agentRow("developer");
    await pick(/filtrar por área/i, "factory");
    await pick(/filtrar por papel/i, "developer");
    await vi.waitFor(() => expect(last(asked)?.get("agentId")).toBe("developer"));

    // Both choices are still valid in the new scope: only clearing drops them.
    choose("acme", "factory");

    await vi.waitFor(() => expect(last(asked)?.get("agentId")).toBeNull());
    expect(last(asked)?.get("area")).toBe("factory");
    expect(screen.getByRole("combobox", { name: /filtrar por papel/i }))
      .toHaveTextContent("Todos os papéis");
  });

  it("never sends an area the new scope does not have", async () => {
    const asked = serve(measures([row()]), AGENTS);
    choose("acme");
    open();
    await agentRow("developer");
    await pick(/filtrar por área/i, "factory");
    await vi.waitFor(() => expect(last(asked)?.get("area")).toBe("factory"));
    const before = measureCalls(asked).length;

    choose("globex");

    await vi.waitFor(() => expect(last(asked)?.get("company")).toBe("globex"));
    const after = measureCalls(asked).slice(before);
    expect(after.map((u) => u.searchParams.get("area"))).not.toContain("factory");
  });

  /** Picks factory and developer in acme, then has the same scope's agents
   *  list come back as `next`. Returns the requests asked from then on. */
  async function refetchAgentsAs(next: ReturnType<typeof agent>[]) {
    const list = [...AGENTS];
    const asked = serve(measures([row()]), list);
    choose("acme");
    const { client } = open();
    await agentRow("developer");
    await pick(/filtrar por área/i, "factory");
    await pick(/filtrar por papel/i, "developer");
    await vi.waitFor(() => expect(last(asked)?.get("agentId")).toBe("developer"));
    const before = measureCalls(asked).length;

    list.splice(0, list.length, ...next);
    await act(() => client.invalidateQueries({ queryKey: ["agents"] }));
    return () => measureCalls(asked).slice(before);
  }

  it("drops an area the same scope's refetched list no longer has, and keeps a role still there", async () => {
    // developer moved to platform; factory is gone.
    const since = await refetchAgentsAs([
      agent("developer", "acme", "platform"),
      agent("analyst", "acme", "finance"),
    ]);

    await vi.waitFor(() => expect(since().at(-1)?.searchParams.get("area")).toBeNull());
    expect(since().at(-1)?.searchParams.get("agentId")).toBe("developer");
    expect(since().map((u) => u.searchParams.get("area"))).not.toContain("factory");
  });

  it("drops a role the same scope's refetched list no longer has, and keeps an area still there", async () => {
    // factory stays, with another role in it; developer is gone.
    const since = await refetchAgentsAs([
      agent("architect", "acme", "factory"),
      agent("analyst", "acme", "finance"),
    ]);

    await vi.waitFor(() => expect(since().at(-1)?.searchParams.get("agentId")).toBeNull());
    expect(since().at(-1)?.searchParams.get("area")).toBe("factory");
    expect(since().map((u) => u.searchParams.get("agentId"))).not.toContain("developer");
  });

  it("lets the active area prevail over a local one", async () => {
    const asked = serve(measures([]), AGENTS);
    choose("acme", "factory");
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );

    renderHook(() => useWorkItemMeasures(8, { area: "finance" }), { wrapper });

    await vi.waitFor(() => expect(last(asked)?.get("area")).toBe("factory"));
  });
});
