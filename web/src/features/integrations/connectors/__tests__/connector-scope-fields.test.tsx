import { render, screen } from "@testing-library/react";
import { useForm, useWatch } from "react-hook-form";
import { beforeEach, describe, expect, it } from "vitest";
import { Form } from "@/components/ui/form";
import { ConnectorIdentityFields, type ConnectorIdentityValues } from "@/features/integrations/connectors/connector-identity-fields";
import { ConnectorScopeContext } from "@/features/integrations/connectors/connector-scope-context";
import { setLocale } from "@/i18n";

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

beforeEach(() => setLocale("pt-BR"));

describe("ConnectorIdentityFields scope", () => {
  it("starts a new instance in the first scope the caller configures, with no free scope", () => {
    render(
      <ConnectorScopeContext.Provider
        value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }, { company: "acme", area: "finance" }] }}
      >
        <Fields />
      </ConnectorScopeContext.Provider>,
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
      <ConnectorScopeContext.Provider value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }] }}>
        <Fields company="acme" area="finance" />
      </ConnectorScopeContext.Provider>,
    );

    expect(screen.getByLabelText("placed")).toHaveTextContent("area acme/cx");
  });

  // A company contains its areas, as the server decides (NT-019).
  const acme = { anyScope: false, configure: [{ company: "acme", area: "" }] };

  it("keeps a new instance in an area of the company the caller configures", () => {
    render(
      <ConnectorScopeContext.Provider value={acme}>
        <Fields company="acme" area="cx" />
      </ConnectorScopeContext.Provider>,
    );

    expect(screen.getByLabelText("placed")).toHaveTextContent("area acme/cx");
    expect(screen.getByRole("combobox")).toHaveTextContent("acme/cx");
  });

  it("shows the stored scope of an instance being edited, though no grant names it", () => {
    render(
      <ConnectorScopeContext.Provider value={acme}>
        <Fields company="acme" area="cx" editing />
      </ConnectorScopeContext.Provider>,
    );

    expect(screen.getByRole("combobox")).toHaveTextContent("acme/cx");
  });

  it("leaves the scope free without a provider, as before", () => {
    render(<Fields />);

    expect(screen.getByText("Escopo da instância")).toBeInTheDocument();
    expect(screen.queryByText("Onde o connector vale")).not.toBeInTheDocument();
  });
});
