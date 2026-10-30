import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthoringPanel } from "@/features/admin/authoring-panel";
import { setLocale } from "@/i18n";

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });

function renderPanel() {
  const puts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      if (input.method === "PUT") {
        puts.push(await input.json());
        return json({});
      }
      if (input.url.includes("/admin/integrations")) {
        return json({
          providers: [{ name: "anthropic", kind: "anthropic" }],
          mcpServers: [],
        });
      }
      return json({
        provider: "anthropic",
        model: "claude-opus-5-5",
        dailyMicros: 5_000_000,
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthoringPanel />
    </QueryClientProvider>,
  );
  return puts;
}

describe("the authoring assistant's daily ceiling", () => {
  beforeEach(() => setLocale("en-US"));

  it("takes a fraction typed with a comma and saves it as micros", async () => {
    const puts = renderPanel();
    const field = await screen.findByLabelText("Daily ceiling");
    await userEvent.clear(field);
    await userEvent.type(field, "1,5");
    expect(field).toHaveValue("1,5");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await vi.waitFor(() =>
      expect(puts).toEqual([
        expect.objectContaining({ dailyMicros: 1_500_000 }),
      ]),
    );
  });
});
