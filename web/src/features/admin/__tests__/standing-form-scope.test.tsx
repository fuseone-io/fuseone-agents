import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StandingForm } from "@/features/admin/standing-form";
import type { Company } from "@/features/companies/api";
import { useActiveScope } from "@/features/scope/active-scope";
import { COMPANIES, pathOf, scopeListsRoute } from "@/test/scope-lists-routes";

// A standing approval holds at a registered company, and one of its areas or
// all of them. A company with no area yet is still a company.

const initech: Company = { id: "initech", label: "Initech", areas: 0, archived: false };

beforeEach(() => {
  useActiveScope.setState({ company: "", area: "" });
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

type Companies = Company[] | "refused" | "failing";

function mount(companies: Companies = [...COMPANIES, initech], fail = false) {
  const sent: { scope: unknown }[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    const listed = scopeListsRoute(request.method, pathOf(request), { companies, fail });
    if (listed) return listed;
    if (request.method === "POST") sent.push(await request.json());
    return new Response(request.method === "POST" ? null : '{"items":[]}', {
      status: request.method === "POST" ? 204 : 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <StandingForm onClose={() => {}} />
    </QueryClientProvider>,
  );
  return async () => {
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Ferramenta"), "crm.close_ticket");
    await user.type(screen.getByLabelText("Agente"), "triage");
    await user.type(screen.getByLabelText("Motivo"), "rotina");
    await user.click(screen.getByRole("button", { name: "Conceder" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    return sent[0]?.scope;
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

describe("where a standing approval holds", () => {
  it("offers every active company, one with no areas included", async () => {
    mount();
    expect(await optionsOf("Empresa")).toEqual(["Acme", "Example", "Initech"]);
  });

  it("offers the whole company and only the chosen company's areas", async () => {
    mount();
    await choose("Empresa", "Acme");
    expect(await optionsOf("Área")).toEqual(["Toda a empresa", "Financeiro", "Operações"]);
  });

  it("opens on the scope the console is reading, and grants there", async () => {
    useActiveScope.setState({ company: "acme", area: "finance" });
    const grant = mount();
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("Financeiro"));
    expect(screen.getByRole("combobox", { name: "Empresa" })).toHaveTextContent("Acme");

    expect(await grant()).toEqual({ company: "acme", area: "finance" });
  });

  it("grants over a whole company with no area", async () => {
    useActiveScope.setState({ company: "acme", area: "finance" });
    const grant = mount();
    await choose("Área", "Toda a empresa");
    expect(await grant()).toEqual({ company: "acme", area: "" });
  });

  it("grants at a company that has no areas yet", async () => {
    const grant = mount();
    await choose("Empresa", "Initech");
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("Toda a empresa");
    expect(await grant()).toEqual({ company: "initech", area: "" });
  });

  it("asks for a company before granting", async () => {
    mount();
    await userEvent.setup().click(screen.getByRole("button", { name: "Conceder" }));
    expect(await screen.findByText("Informe a empresa.")).toBeInTheDocument();
  });

  // Granting needs no authority over the installation; listing companies does.
  it("offers the areas' companies when the company list is refused", async () => {
    mount("refused");
    expect(await optionsOf("Empresa")).toEqual(["acme", "example"]);
  });

  it("says the lists failed, offers a retry, and keeps the active scope", async () => {
    useActiveScope.setState({ company: "acme", area: "finance" });
    mount(undefined, true);

    expect(await screen.findByText(/Não foi possível carregar as empresas e áreas/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Empresa" })).toHaveTextContent("acme");
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("finance");
  });

  // Somebody who grants without authority over the installation is refused
  // the company list by design: nothing failed, and a retry would be refused
  // again.
  it("takes a refused company list as no failure, with no retry", async () => {
    mount("refused");
    await waitFor(() => expect(document.querySelector('[data-slot="skeleton"]')).toBeNull());

    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("button", { name: "Tentar de novo" })).toBeNull();
  });

  it("says a company list that failed failed, with a retry", async () => {
    mount("failing");

    expect(await screen.findByText(/Não foi possível carregar as empresas e áreas/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
  });
});
