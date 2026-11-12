import { render, screen } from "@testing-library/react";
import { useForm } from "react-hook-form";
import { beforeEach, describe, expect, it } from "vitest";
import { Form } from "@/components/ui/form";
import { ConnectorIdentityFields, type ConnectorIdentityValues } from "@/features/integrations/connectors/connector-identity-fields";
import { ConnectorScopeContext } from "@/features/integrations/connectors/connector-scope-context";
import { setLocale } from "@/i18n";

// NT-019: an area's administrator places an instance in a scope where it
// configures, never in one typed freely that the server would refuse.

function Fields() {
  const form = useForm<ConnectorIdentityValues>({
    defaultValues: { name: "", enabled: true, scopeKind: "area", company: "", area: "" },
  });
  return (
    <Form {...form}>
      <ConnectorIdentityFields editing={false} connector="vault" />
      <output aria-label="placed">{`${form.watch("scopeKind")} ${form.watch("company")}/${form.watch("area")}`}</output>
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

  it("leaves the scope free without a provider, as before", () => {
    render(<Fields />);

    expect(screen.getByText("Escopo da instância")).toBeInTheDocument();
    expect(screen.queryByText("Onde o connector vale")).not.toBeInTheDocument();
  });
});
