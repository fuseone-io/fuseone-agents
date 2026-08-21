import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentAreaField } from "@/features/agents/agent-area-field";
import { setLocale } from "@/i18n";

function stubScopes() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      new Response(
        JSON.stringify({
          items: [
            { company: "default", area: "platform", label: "Default platform" },
            { company: "acme", area: "platform", label: "Acme platform" },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    ),
  );
}

function renderField(onChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AgentAreaField company="" area="" onChange={onChange} />
    </QueryClientProvider>,
  );
  return onChange;
}

describe("the agent area field", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  });

  beforeEach(() => {
    vi.restoreAllMocks();
    setLocale("en-US");
    stubScopes();
  });

  it("keeps company with the area when two companies use the same area id", async () => {
    const user = userEvent.setup();
    const onChange = renderField();

    await user.click(screen.getByRole("combobox", { name: "Area" }));
    await user.click(await screen.findByRole("option", { name: /acme\/platform/ }));

    expect(onChange).toHaveBeenCalledWith({ company: "acme", area: "platform" });
  });
});
