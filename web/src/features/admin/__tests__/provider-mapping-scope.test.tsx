import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GroupMapping } from "@/features/admin/identity-api";
import { IdentityProviderForm } from "@/features/admin/provider-identity-form";
import { pathOf, scopeListsRoute } from "@/test/scope-lists-routes";

// Where a group mapping grants its role is chosen from what is registered;
// the group itself is the identity provider's, and stays typed.

beforeEach(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

function mount(mappings: GroupMapping[]) {
  const sent: unknown[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    const listed = scopeListsRoute(request.method, pathOf(request));
    if (listed) return listed;
    if (request.method === "PUT") sent.push(await request.json());
    return new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } });
  });
  const provider = { id: "corp", display: "Corp", issuer: "https://sso.example.com", hasSecret: true, enabled: true, mappings };
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <IdentityProviderForm provider={provider} onDone={() => {}} />
    </QueryClientProvider>,
  );
  return async () => {
    await userEvent.setup().click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    return (sent[0] as { mappings: GroupMapping[] }).mappings;
  };
}

async function optionsOf(name: string) {
  await waitFor(() => expect(screen.getByRole("combobox", { name })).toBeEnabled());
  await userEvent.setup().click(screen.getByRole("combobox", { name }));
  return screen.getAllByRole("option").map((option) => option.textContent);
}

async function choose(name: string, option: string) {
  await optionsOf(name);
  await userEvent.setup().click(screen.getByRole("option", { name: option }));
}

const mapping = (company: string, area: string, role: GroupMapping["role"] = "author"): GroupMapping => ({
  group: "sre", company, area, role,
});

describe("where a group mapping applies", () => {
  it("offers the registered companies, never a withdrawn one", async () => {
    mount([mapping("example", "cx")]);
    expect(await optionsOf("Empresa")).toEqual(["Acme", "Example"]);
  });

  it("offers the whole company and only the chosen company's areas", async () => {
    mount([mapping("example", "cx")]);
    await choose("Empresa", "Acme");
    expect(await optionsOf("Área")).toEqual(["Toda a empresa", "Financeiro", "Operações"]);
  });

  it("saves the company and area chosen, with the group as typed", async () => {
    const save = mount([mapping("example", "cx")]);
    await userEvent.setup().type(screen.getByLabelText("Grupo"), "-oncall");
    await choose("Empresa", "Acme");
    await choose("Área", "Operações");

    expect(await save()).toEqual([{ group: "sre-oncall", company: "acme", area: "ops", role: "author" }]);
  });

  it("saves a company-wide mapping with no area", async () => {
    const save = mount([mapping("acme", "finance")]);
    await choose("Área", "Toda a empresa");
    expect(await save()).toEqual([mapping("acme", "")]);
  });
});

describe("a mapping the lists do not choose", () => {
  it("keeps the installation administrator mapping as it is", async () => {
    const save = mount([mapping("*", "", "admin")]);
    expect(await screen.findByRole("combobox", { name: "Empresa" })).toHaveTextContent("Toda a instalação");
    expect(await save()).toEqual([mapping("*", "", "admin")]);
  });

  it("shows a company and an area nobody registered, and saves them unchanged", async () => {
    const save = mount([mapping("legacy", "gone")]);
    const company = screen.getByRole("combobox", { name: "Empresa" });
    await waitFor(() => expect(company).toHaveTextContent("legacy (não registrada)"));
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("gone (não registrada)");
    expect(await save()).toEqual([mapping("legacy", "gone")]);
  });
});
