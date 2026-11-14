import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PriceForm } from "@/features/admin/price-form";
import { setLocale } from "@/i18n";

/** The providers an author picks from, and the price writes, at the network. */
function stubNetwork() {
  const puts: unknown[] = [];
  const items = [
    {
      name: "azure-gpt",
      kind: "openai_compatible",
      models: ["gpt-5-mini"],
      usable: true,
    },
  ];
  vi.stubGlobal("fetch", async (request: Request) => {
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    if (request.method === "PUT") puts.push(await request.json());
    if (path === "/providers") return Response.json({ items });
    return new Response(null, { status: path === "/admin/prices" ? 204 : 404 });
  });
  return puts;
}

function renderNewPrice() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const known = [{ provider: "anthropic", model: "claude-opus-5" }];
  render(
    <QueryClientProvider client={client}>
      <PriceForm price={null} knownPrices={known} onClose={() => {}} />
    </QueryClientProvider>,
  );
}

const user = () => userEvent.setup();
const save = (u: ReturnType<typeof user>) =>
  u.click(screen.getByRole("button", { name: "Save" }));

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
});
afterEach(() => vi.unstubAllGlobals());

describe("a new price's provider and model", () => {
  // A rate is set for a provider the installation reaches, so its name is
  // offered even before any price names it.
  it("offers the connected providers beside the priced ones", async () => {
    stubNetwork();
    renderNewPrice();
    const u = user();
    await u.click(screen.getByRole("combobox", { name: "Known providers" }));
    expect(await screen.findByText("anthropic")).toBeInTheDocument();
    await u.click(await screen.findByText("azure-gpt"));
    expect(screen.getByLabelText("Provider")).toHaveValue("azure-gpt");
  });

  // Picking writes what typing the same names would have written.
  it("offers the chosen provider's models and saves the pair", async () => {
    const puts = stubNetwork();
    renderNewPrice();
    const u = user();
    await u.type(screen.getByLabelText("Provider"), "azure-gpt");
    await u.click(screen.getByRole("combobox", { name: "Known models" }));
    await u.click(await screen.findByText("gpt-5-mini"));
    await save(u);
    await waitFor(() =>
      expect(puts).toEqual([
        expect.objectContaining({ provider: "azure-gpt", model: "gpt-5-mini" }),
      ]),
    );
  });

  // Prices may be set ahead of use: names nothing lists still save.
  it("saves a provider and model nothing lists yet", async () => {
    const puts = stubNetwork();
    renderNewPrice();
    const u = user();
    await u.type(screen.getByLabelText("Provider"), "future-ai");
    await u.type(screen.getByLabelText("Model"), "next-1");
    await save(u);
    await waitFor(() =>
      expect(puts).toEqual([
        expect.objectContaining({ provider: "future-ai", model: "next-1" }),
      ]),
    );
  });
});
