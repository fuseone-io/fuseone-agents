import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConditionBuilder } from "@/features/policies/condition-builder";
import { setLocale } from "@/i18n";

/** The tool catalogue at the network boundary; every path asked is kept. */
function stubTools() {
  const asked: string[] = [];
  const items = [{ toolId: "cloudflare.edge.block_ip", effect: "write" }];
  vi.stubGlobal("fetch", async (request: Request) => {
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    asked.push(path);
    const body = path === "/admin/tools" ? { items } : {};
    return new Response(JSON.stringify(body), {
      status: path === "/admin/tools" ? 200 : 404,
      headers: { "Content-Type": "application/json" },
    });
  });
  return asked;
}

function renderToolRow(onChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <ConditionBuilder
        conditions={[{ field: "tool.id", op: "eq", value: "" }]}
        onChange={onChange}
      />
    </QueryClientProvider>,
  );
  return onChange;
}

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
});
afterEach(() => vi.unstubAllGlobals());

describe("a tool.id condition value", () => {
  // The same listing the policy's own Tool field offers, picked into the
  // value exactly as typing the id would have written it.
  it("offers the governed tools", async () => {
    stubTools();
    const onChange = renderToolRow();
    await userEvent.click(
      screen.getByRole("combobox", { name: "Suggest known values" }),
    );
    await userEvent.click(await screen.findByText("cloudflare.edge.block_ip"));
    expect(onChange).toHaveBeenCalledWith([
      { field: "tool.id", op: "eq", value: "cloudflare.edge.block_ip" },
    ]);
  });

  // The page is visible to roles the listing refuses, so it is asked for
  // only when somebody opens the suggestions.
  it("asks for the catalogue only when the suggestions open", async () => {
    const asked = stubTools();
    renderToolRow();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(asked).not.toContain("/admin/tools");
  });

  // A glob is a legitimate rule and no listing holds it: one keystroke
  // must still arrive, or the field stopped accepting free text.
  it("still accepts a glob nothing lists", async () => {
    stubTools();
    const onChange = renderToolRow();
    await userEvent.type(screen.getByLabelText("Condition 1 value"), "*");
    expect(onChange).toHaveBeenCalledWith([
      { field: "tool.id", op: "eq", value: "*" },
    ]);
  });
});
