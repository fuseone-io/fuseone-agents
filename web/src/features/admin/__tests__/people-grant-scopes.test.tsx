import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  scopeListsRoute,
  pathOf,
} from "@/test/scope-lists-routes";
import { GrantEditor } from "@/features/admin/grant-editor";
import type { GrantInput, Person } from "@/features/admin/people-api";

type Sent = { method: string; path: string; body?: unknown };

beforeEach(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

function stub(lists: { fail?: boolean } = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = pathOf(request);
      const listed = scopeListsRoute(request.method, path, lists);
      if (listed) return listed;
      const text = await request.clone().text();
      sent.push({ method: request.method, path, body: text && JSON.parse(text) });
      return new Response(null, { status: 204 });
    }),
  );
  return sent;
}

function mount(granted: GrantInput[] = []) {
  const person: Person = {
    id: "usr_ana",
    kind: "user",
    display: "Ana",
    disabled: false,
    grants: granted.map((g) => ({ ...g, asserted: false })),
  };
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <GrantEditor person={person} onDone={() => {}} />
    </QueryClientProvider>,
  );
}

async function choose(name: RegExp | string, option: string) {
  const user = userEvent.setup();
  await user.click(screen.getAllByRole("combobox", { name }).at(-1)!);
  await user.click(await screen.findByRole("option", { name: option }));
}

describe("where a person's grant applies", () => {
  it("offers the active companies, never a withdrawn one, for a new grant", async () => {
    stub();
    mount();
    await userEvent.click(screen.getByRole("button", { name: "Adicionar acesso" }));
    await userEvent.click(await screen.findByRole("combobox", { name: "Empresa" }));

    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getByRole("option", { name: "Acme" })).toBeInTheDocument();
    expect(within(listbox).getByRole("option", { name: "Example" })).toBeInTheDocument();
    expect(within(listbox).queryByRole("option", { name: /Globex/ })).toBeNull();
  });

  it("offers the whole company and only the chosen company's areas", async () => {
    stub();
    mount([{ company: "example", area: "cx", role: "author" }]);
    await choose("Empresa", "Acme");
    await userEvent.click(screen.getByRole("combobox", { name: "Área" }));

    const names = within(await screen.findByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(names).toEqual(["Toda a empresa", "Financeiro", "Operações"]);
  });

  it("saves the company and area chosen as the grant the server expects", async () => {
    const sent = stub();
    mount([{ company: "example", area: "cx", role: "approver" }]);
    await choose("Empresa", "Acme");
    await choose("Área", "Operações");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({
      method: "PUT",
      path: "/admin/people/usr_ana/grants",
      body: { grants: [{ company: "acme", area: "ops", role: "approver" }] },
    });
  });

  it("saves a grant over the whole company with no area", async () => {
    const sent = stub();
    mount([{ company: "acme", area: "finance", role: "auditor" }]);
    await choose("Área", "Toda a empresa");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.body).toEqual({
      grants: [{ company: "acme", area: "", role: "auditor" }],
    });
  });
});

describe("a grant the lists do not know", () => {
  it("still shows a withdrawn company and an unregistered area, and saves them unchanged", async () => {
    const sent = stub();
    mount([{ company: "globex", area: "legacy", role: "author" }]);

    const company = await screen.findByRole("combobox", { name: "Empresa" });
    await waitFor(() => expect(company).toHaveTextContent("Globex (retirada)"));
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("legacy (não registrada)");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.body).toEqual({
      grants: [{ company: "globex", area: "legacy", role: "author" }],
    });
  });

  it("says the lists failed, offers a retry, and keeps the grant", async () => {
    stub({ fail: true });
    mount([{ company: "acme", area: "finance", role: "author" }]);

    expect(await screen.findByText(/Não foi possível carregar as empresas e áreas/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Empresa" })).toHaveTextContent("acme");
    expect(screen.getByRole("combobox", { name: "Área" })).toHaveTextContent("finance");
  });

  it("keeps the installation-wide administrator grant as it is", async () => {
    const sent = stub();
    mount([{ company: "*", area: "", role: "admin" }]);

    expect(await screen.findByRole("combobox", { name: "Empresa" })).toHaveTextContent("Toda a instalação");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.body).toEqual({ grants: [{ company: "*", area: "", role: "admin" }] });
  });
});
