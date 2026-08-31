import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  ConnectorInstance,
  ConnectorInstanceDetail,
} from "@/features/integrations/api";
import { SQLInstanceForm } from "@/features/integrations/connectors/sql-instance-form";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";

const vault: ConnectorInstance = {
  name: "prod",
  connector: "vault",
  enabled: true,
  scopeKind: "company",
  company: "acme",
  hasToken: true,
  vault: {
    address: "https://vault.internal",
    mount: "secret",
    allowedPathPrefixes: ["database/creds"],
  },
};

const sqlInstance: ConnectorInstanceDetail = {
  name: "app-x",
  connector: "sql",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: false,
  sql: {
    driver: "postgres",
    host: "db.internal",
    port: 5432,
    database: "appx",
    credentialSource: {
      kind: "vault_database_role",
      vaultInstance: "prod",
      mount: "database",
      role: "app-x-readonly",
    },
    templates: [{
      id: "",
      sql: "",
      parameters: [],
      timeoutSeconds: 30,
      maxRows: 200,
      maxBytes: 65_536,
    }],
  },
};

beforeAll(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.setPointerCapture ??= () => {};
  Element.prototype.releasePointerCapture ??= () => {};
  Element.prototype.scrollIntoView ??= () => {};
});

beforeEach(() => {
  setLocale("pt-BR");
  useActiveScope.setState({ company: "acme", area: "platform" });
});

describe("SQL connector instance form", () => {
  it("submits a governed template without accepting a DSN or credential", async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => undefined);
    render(
      <SQLInstanceForm
        instance={sqlInstance}
        instances={[vault]}
        onClose={vi.fn()}
        onSave={save}
      />,
    );

    expect(screen.getByLabelText("Nome da instância")).toBeDisabled();
    expect(
      screen.getByRole("combobox", { name: "Escopo da instância" }),
    ).toBeDisabled();
    expect(screen.getByLabelText("Empresa")).toBeDisabled();
    expect(screen.getByLabelText("Área")).toBeDisabled();
    await user.type(screen.getByLabelText("Id do template"), "lookup");
    await user.type(
      screen.getByLabelText("Query registrada"),
      "select $1::text as echo",
    );
    await user.click(screen.getByRole("button", { name: "Adicionar parâmetro" }));
    await user.type(screen.getByRole("textbox", { name: "Nome do parâmetro 1" }), "message");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith({
      name: "app-x",
      body: {
        connector: "sql",
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        sql: {
          driver: "postgres",
          host: "db.internal",
          port: 5432,
          database: "appx",
          credentialSource: {
            kind: "vault_database_role",
            vaultInstance: "prod",
            mount: "database",
            role: "app-x-readonly",
          },
          templates: [{
            id: "lookup",
            sql: "select $1::text as echo",
            parameters: [{ name: "message", type: "text" }],
            timeoutSeconds: 30,
            maxRows: 200,
            maxBytes: 65_536,
          }],
        },
      },
    });
    expect(screen.queryByLabelText(/password|senha|dsn/i)).not.toBeInTheDocument();
  });

  it("offers the compatible Vault binding by its governed name", async () => {
    const user = userEvent.setup();
    render(
      <SQLInstanceForm
        instance={null}
        instances={[vault]}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Instância Vault" }));
    expect(
      await screen.findByRole("option", { name: "prod · acme" }),
    ).toBeInTheDocument();
  });

  it("explains binding failures", () => {
    render(
      <SQLInstanceForm
        instance={null}
        instances={[{ ...vault, hasToken: false }]}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("Nenhum Vault compatível");
    expect(screen.getByText(/HTTPS, ativa e com token/)).toBeInTheDocument();
  });
});
