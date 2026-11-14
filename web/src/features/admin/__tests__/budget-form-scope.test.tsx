import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ScopeBudget } from "@/features/admin/api";
import { BudgetForm } from "@/features/admin/budget-form";
import { pathOf, scopeListsRoute } from "@/test/scope-lists-routes";

// A ceiling applies to the installation, a company or an area, chosen from
// what is registered, and still reaches the API as `installation`, `acme` or
// `acme/cx`.

beforeEach(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

function stub(lists: Parameters<typeof scopeListsRoute>[2] = {}) {
  const puts: string[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    const path = pathOf(request);
    const listed = scopeListsRoute(request.method, path, lists);
    if (listed) return listed;
    if (request.method === "PUT") puts.push(decodeURIComponent(path));
    return new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } });
  });
  return puts;
}

function mount(budget: ScopeBudget | null = null) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BudgetForm budget={budget} onClose={() => {}} />
    </QueryClientProvider>,
  );
}

async function choose(name: string, option: string) {
  const user = userEvent.setup();
  await waitFor(() => expect(screen.getByRole("combobox", { name })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name }));
  await user.click(await screen.findByRole("option", { name: option }));
}

async function optionsOf(name: string) {
  await waitFor(() => expect(screen.getByRole("combobox", { name })).toBeEnabled());
  await userEvent.setup().click(screen.getByRole("combobox", { name }));
  return screen.getAllByRole("option").map((option) => option.textContent);
}

async function save() {
  await userEvent.setup().click(screen.getByRole("button", { name: "Salvar" }));
}

describe("what a new ceiling applies to", () => {
  it("offers the installation and the registered companies, never a withdrawn one", async () => {
    stub();
    mount();
    expect(await optionsOf("Empresa")).toEqual(["Toda a instalação", "Acme", "Example"]);
  });

  it("offers the whole company and only the chosen company's areas", async () => {
    stub();
    mount();
    await choose("Empresa", "Acme");
    expect(await optionsOf("Área")).toEqual(["Toda a empresa", "Financeiro", "Operações"]);
  });

  it.each([
    ["an area", "Acme", "Operações", "/admin/budgets/acme/ops"],
    ["a whole company", "Example", undefined, "/admin/budgets/example"],
    ["the installation", "Toda a instalação", undefined, "/admin/budgets/installation"],
  ])("sends %s in the path the API reads", async (_, company, area, path) => {
    const puts = stub();
    mount();
    await choose("Empresa", company);
    if (area) await choose("Área", area);
    await save();

    await waitFor(() => expect(puts).toEqual([path]));
  });

  it("asks what the ceiling applies to before saving", async () => {
    const puts = stub();
    mount();
    await save();

    expect(await screen.findByText("Diga a que este teto se aplica.")).toBeInTheDocument();
    expect(puts).toEqual([]);
  });
});

describe("a ceiling being edited", () => {
  const ceiling = (company: string, area: string): ScopeBudget => ({
    scopeKind: area ? "area" : company ? "company" : "installation",
    scope: { company, area },
    period: "monthly",
    enabled: true,
  });

  it("shows a scope the lists do not know, locked, and saves it unchanged", async () => {
    const puts = stub();
    mount(ceiling("legacy", "gone"));
    const company = screen.getByRole("combobox", { name: "Empresa" });
    await waitFor(() => expect(company).toHaveTextContent("legacy (não registrada)"));
    expect(company).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("gone (não registrada)");
    await save();

    await waitFor(() => expect(puts).toEqual(["/admin/budgets/legacy/gone"]));
  });

  it("shows the installation's ceiling as the installation", async () => {
    stub();
    mount(ceiling("", ""));
    expect(await screen.findByRole("combobox", { name: "Empresa" })).toHaveTextContent("Toda a instalação");
  });

  it("says the lists failed, offers a retry, and keeps the ceiling's scope", async () => {
    stub({ fail: true });
    mount(ceiling("acme", "cx"));

    expect(await screen.findByText(/Não foi possível carregar as empresas e áreas/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Empresa" })).toHaveTextContent("acme");
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("cx");
  });

  it("still shows a ceiling's scope when the company list is refused", async () => {
    stub({ companies: "refused" });
    mount(ceiling("", ""));
    expect(await screen.findByRole("textbox", { name: "Empresa" })).toHaveValue("*");
  });
});

describe("a new ceiling when the company list is refused", () => {
  it.each([
    ["a company no area names", "initech", undefined, "/admin/budgets/initech"],
    ["an area of it", "initech", "legacy", "/admin/budgets/initech/legacy"],
  ])("takes %s typed", async (_, company, area, path) => {
    const puts = stub({ companies: "refused" });
    mount();
    const user = userEvent.setup();
    await user.type(await screen.findByRole("textbox", { name: "Empresa" }), company);
    if (area) await user.type(screen.getByRole("textbox", { name: "Área" }), area);
    await save();
    await waitFor(() => expect(puts).toEqual([path]));
  });

  it("offers the installation among the suggestions", async () => {
    const puts = stub({ companies: "refused" });
    mount();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("combobox", { name: "Sugerir empresas conhecidas" }));
    await user.click(screen.getByRole("option", { name: "Toda a instalação" }));
    await save();
    await waitFor(() => expect(puts).toEqual(["/admin/budgets/installation"]));
  });
});
