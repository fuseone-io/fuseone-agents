import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderBare, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { useForm, useWatch } from "react-hook-form";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Form } from "@/components/ui/form";
import { ConnectorIdentityFields, type ConnectorIdentityValues } from "@/features/integrations/connectors/connector-identity-fields";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";
import { setLocale } from "@/i18n";
import { pathOf, scopeListsRoute } from "@/test/scope-lists-routes";

// The free scope reads the registered companies and areas.
const render = (ui: ReactElement) =>
  renderBare(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {ui}
    </QueryClientProvider>,
  );

// NT-019: an area's administrator places an instance in a scope where it
// configures, never in one typed freely that the server would refuse.

function Fields({ company = "", area = "", editing = false }: { company?: string; area?: string; editing?: boolean }) {
  const form = useForm<ConnectorIdentityValues>({
    defaultValues: { name: "", enabled: true, scopeKind: area ? "area" : "company", company, area },
  });
  const placed = useWatch({ control: form.control });
  return (
    <Form {...form}>
      <ConnectorIdentityFields editing={editing} connector="vault" />
      <output aria-label="placed">{`${placed.scopeKind} ${placed.company}/${placed.area}`}</output>
    </Form>
  );
}

beforeEach(() => {
  setLocale("pt-BR");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

async function choose(name: string, option: string) {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("combobox", { name }));
  await user.click(await screen.findByRole("option", { name: option }));
}

describe("ConnectorIdentityFields scope", () => {
  it("starts a new instance in the first scope the caller configures, with no free scope", () => {
    render(
      <ScopeChoiceContext.Provider
        value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }, { company: "acme", area: "finance" }] }}
      >
        <Fields />
      </ScopeChoiceContext.Provider>,
    );

    expect(screen.getByText("Onde o connector vale")).toBeInTheDocument();
    expect(screen.getByLabelText("placed")).toHaveTextContent("area acme/cx");
    expect(screen.queryByText("Escopo da instância")).not.toBeInTheDocument();
  });

  // The scope a new form opens on can come from somewhere else, such as the
  // area the caller is working in under another grant. Kept, it would be saved
  // into a scope the server refuses.
  it("moves a new instance off a scope the caller does not configure", () => {
    render(
      <ScopeChoiceContext.Provider value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }] }}>
        <Fields company="acme" area="finance" />
      </ScopeChoiceContext.Provider>,
    );

    expect(screen.getByLabelText("placed")).toHaveTextContent("area acme/cx");
  });

  // A company contains its areas, as the server decides (NT-019).
  const acme = { anyScope: false, configure: [{ company: "acme", area: "" }] };

  it("keeps a new instance in an area of the company the caller configures", () => {
    render(
      <ScopeChoiceContext.Provider value={acme}>
        <Fields company="acme" area="cx" />
      </ScopeChoiceContext.Provider>,
    );

    expect(screen.getByLabelText("placed")).toHaveTextContent("area acme/cx");
    expect(screen.getByRole("combobox")).toHaveTextContent("acme/cx");
  });

  it("shows the stored scope of an instance being edited, though no grant names it", () => {
    render(
      <ScopeChoiceContext.Provider value={acme}>
        <Fields company="acme" area="cx" editing />
      </ScopeChoiceContext.Provider>,
    );

    expect(screen.getByRole("combobox")).toHaveTextContent("acme/cx");
  });

  it("leaves the scope free without a provider, as before", () => {
    render(<Fields />);

    expect(screen.getByText("Escopo da instância")).toBeInTheDocument();
    expect(screen.queryByText("Onde o connector vale")).not.toBeInTheDocument();
  });

  it("places a free instance at a company and an area chosen from the lists", async () => {
    vi.stubGlobal("fetch", async (request: Request) =>
      scopeListsRoute(request.method, pathOf(request)) ?? new Response(null, { status: 404 }),
    );
    render(<Fields />);
    await choose("Escopo da instância", "Área");
    await choose("Empresa", "Acme");
    await choose("Área", "Operações");

    expect(screen.getByLabelText("placed")).toHaveTextContent("area acme/ops");
  });
});
