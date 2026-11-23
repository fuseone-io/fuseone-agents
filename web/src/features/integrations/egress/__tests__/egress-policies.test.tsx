import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EgressPoliciesPanel } from "@/features/integrations/egress/egress-policies-panel";
import { pathOf, scopeListsRoute } from "@/test/scope-lists-routes";

// Where a company's or an area's MCP servers may send requests (NT-019): one
// list of exact origins per scope, written by the platform alone.

type Sent = { method: string; url: string; body?: unknown; ifMatch: string | null; ifNoneMatch: string | null };

beforeEach(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

const stored = [
  { company: "acme", area: "finance", origins: ["https://jira.example.com", "https://wiki.example.com:8443"], etag: '"e-finance"' },
  { company: "example", origins: [], etag: '"e-example"' },
];

function mount(policies: object[] = stored, write?: Response) {
  const sent: Sent[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    const listed = scopeListsRoute(request.method, pathOf(request));
    if (listed) return listed;
    if (request.method === "GET") {
      return new Response(JSON.stringify({ policies }), { headers: { "Content-Type": "application/json" } });
    }
    const text = await request.text();
    sent.push({
      method: request.method, url: request.url, body: text ? JSON.parse(text) : undefined,
      ifMatch: request.headers.get("If-Match"), ifNoneMatch: request.headers.get("If-None-Match"),
    });
    return write ?? new Response(null, { status: 204 });
  });
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <EgressPoliciesPanel />
    </QueryClientProvider>,
  );
  return sent;
}

describe("the MCP egress policies", () => {
  it("lists each scope with the origins it allows, and an empty list as reaching nothing", async () => {
    mount();
    const finance = (await screen.findByText("acme/finance")).closest("li") as HTMLElement;
    expect(within(finance).getByText("https://jira.example.com")).toBeInTheDocument();
    expect(within(finance).getByText("https://wiki.example.com:8443")).toBeInTheDocument();
    const example = screen.getByText("example").closest("li") as HTMLElement;
    expect(within(example).getByText("Não alcança nada")).toBeInTheDocument();
  });

  it("says what a policy is for when there is none", async () => {
    mount([]);
    expect(await screen.findByText("Nenhuma política de egress")).toBeInTheDocument();
  });

  it("stores a policy for an area, one origin per line", async () => {
    const sent = mount([]);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Nova política" }));
    await user.click(await screen.findByRole("combobox", { name: "Empresa" }));
    await user.click(screen.getByRole("option", { name: "Acme" }));
    await user.click(screen.getByRole("combobox", { name: "Área" }));
    await user.click(screen.getByRole("option", { name: "Operações" }));
    await user.type(screen.getByLabelText("Origens permitidas"), "https://grafana.example.com\n\n https://zen.example.com ");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toMatchObject({ method: "PUT", body: {
      company: "acme", area: "ops", origins: ["https://grafana.example.com", "https://zen.example.com"],
    } });
  });

  it("edits a policy from what is stored", async () => {
    const sent = mount();
    const user = userEvent.setup();
    const finance = (await screen.findByText("acme/finance")).closest("li") as HTMLElement;
    await user.click(within(finance).getByRole("button", { name: "Editar" }));
    expect(screen.getByLabelText("Origens permitidas")).toHaveValue("https://jira.example.com\nhttps://wiki.example.com:8443");
    await user.type(screen.getByLabelText("Origens permitidas"), "\nhttps://ci.example.com");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect((sent[0]?.body as { origins: string[] }).origins).toEqual([
      "https://jira.example.com", "https://wiki.example.com:8443", "https://ci.example.com",
    ]);
  });

  it("removes a policy only once confirmed, by its scope", async () => {
    const sent = mount();
    const user = userEvent.setup();
    const finance = (await screen.findByText("acme/finance")).closest("li") as HTMLElement;
    await user.click(within(finance).getByRole("button", { name: "Remover" }));
    expect(sent).toEqual([]);
    await user.click(await screen.findByRole("button", { name: "Remover política" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    const url = new URL(sent[0]!.url);
    expect([sent[0]!.method, url.searchParams.get("company"), url.searchParams.get("area")]).toEqual(["DELETE", "acme", "finance"]);
  });

  it("says why the server refused an origin, and keeps the dialog open", async () => {
    const refused = new Response(JSON.stringify({ status: 400, title: "Invalid request", type: "fuseone:invalid-input", detail: "not an origin: https://x.example.com/path" }),
      { status: 400, headers: { "Content-Type": "application/problem+json" } });
    mount([], refused);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Nova política" }));
    await user.click(await screen.findByRole("combobox", { name: "Empresa" }));
    await user.click(screen.getByRole("option", { name: "Acme" }));
    await user.type(screen.getByLabelText("Origens permitidas"), "https://x.example.com/path");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/not an origin/);
    expect(screen.getByRole("button", { name: "Salvar" })).toBeInTheDocument();
  });
  // A policy is replaced whole: a write that did not see what is stored erases it.
  describe("never replaces what it did not read", () => {
    async function newPolicyFor(user: ReturnType<typeof userEvent.setup>, area?: string) {
      await user.click(await screen.findByRole("button", { name: "Nova política" }));
      await user.click(await screen.findByRole("combobox", { name: "Empresa" }));
      await user.click(screen.getByRole("option", { name: "Acme" }));
      if (area) {
        await user.click(screen.getByRole("combobox", { name: "Área" }));
        await user.click(screen.getByRole("option", { name: area }));
      }
    }

    it("creates a policy as a creation, refused by the server if the scope has one", async () => {
      const sent = mount([]);
      const user = userEvent.setup();
      await newPolicyFor(user);
      await user.type(screen.getByLabelText("Origens permitidas"), "https://jira.example.com");
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      await waitFor(() => expect(sent).toHaveLength(1));
      expect(sent[0]).toMatchObject({ ifNoneMatch: "*", ifMatch: null });
    });

    it("opens, for a scope that has a policy, that policy with its origins rather than an empty one", async () => {
      const sent = mount([{ company: "acme", origins: ["https://jira.example.com"], etag: '"e-acme"' }]);
      const user = userEvent.setup();
      await newPolicyFor(user);
      expect(await screen.findByText(/já tem uma política/)).toBeInTheDocument();
      expect(screen.getByLabelText("Origens permitidas")).toHaveValue("https://jira.example.com");
      await user.type(screen.getByLabelText("Origens permitidas"), "\nhttps://crm.example.com");
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      await waitFor(() => expect(sent).toHaveLength(1));
      expect(sent[0]).toMatchObject({ ifMatch: '"e-acme"', ifNoneMatch: null, body: { origins: ["https://jira.example.com", "https://crm.example.com"] } });
    });

    it("takes the loaded origins back out when the scope chosen moves to one with no policy", async () => {
      const sent = mount([{ company: "acme", origins: ["https://jira.example.com"], etag: '"e-acme"' }]);
      const user = userEvent.setup();
      await newPolicyFor(user);
      await user.type(screen.getByLabelText("Origens permitidas"), "\nhttps://crm.example.com");
      await user.click(screen.getByRole("combobox", { name: "Área" }));
      await user.click(screen.getByRole("option", { name: "Operações" }));
      expect(screen.getByLabelText("Origens permitidas")).toHaveValue("https://crm.example.com");
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      await waitFor(() => expect(sent).toHaveLength(1));
      expect(sent[0]).toMatchObject({ ifNoneMatch: "*", body: { area: "ops", origins: ["https://crm.example.com"] } });
    });

    it("edits and removes against the revision it read", async () => {
      const sent = mount();
      const user = userEvent.setup();
      const finance = (await screen.findByText("acme/finance")).closest("li") as HTMLElement;
      await user.click(within(finance).getByRole("button", { name: "Editar" }));
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      await waitFor(() => expect(sent).toHaveLength(1));
      await user.click(within(finance).getByRole("button", { name: "Remover" }));
      await user.click(await screen.findByRole("button", { name: "Remover política" }));
      await waitFor(() => expect(sent).toHaveLength(2));
      expect(sent.map((one) => one.ifMatch)).toEqual(['"e-finance"', '"e-finance"']);
    });

    it("says the policy changed when the server refuses the write as stale, and keeps the dialog open", async () => {
      const stale = new Response(JSON.stringify({ status: 412, title: "Precondition failed", type: "fuseone:precondition-failed" }),
        { status: 412, headers: { "Content-Type": "application/problem+json" } });
      mount(stored, stale);
      const user = userEvent.setup();
      const finance = (await screen.findByText("acme/finance")).closest("li") as HTMLElement;
      await user.click(within(finance).getByRole("button", { name: "Editar" }));
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      expect(await screen.findByRole("alert")).toHaveTextContent(/mudou desde que você a abriu/);
      expect(screen.getByRole("button", { name: "Salvar" })).toBeInTheDocument();
    });
  });
});
