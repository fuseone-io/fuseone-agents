import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  ConnectorInstance,
  ConnectorInstanceDetail,
} from "@/features/integrations/api";
import { GraviteeInstanceForm } from "@/features/integrations/connectors/gravitee-instance-form";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";

const vault: ConnectorInstance = {
  name: "secrets",
  connector: "vault",
  enabled: true,
  scopeKind: "company",
  company: "acme",
  hasToken: true,
  vault: {
    address: "https://vault.internal",
    mount: "secret",
    allowedPathPrefixes: ["integrations/gravitee"],
  },
};

const gravitee: ConnectorInstanceDetail = {
  name: "apim",
  connector: "gravitee",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: false,
  gravitee: {
    address: "https://apim.internal/management/v2",
    organization: "org-prod",
    environment: "env-prod",
    allowedReferences: [{ type: "API", id: "checkout-api" }],
    minTTLSeconds: 86_400,
    maxTTLSeconds: 7_776_000,
    allowNoExpiry: false,
    credentialSource: {
      kind: "vault_kv_secret",
      vaultInstance: "secrets",
      path: "integrations/gravitee/prod",
      field: "access_token",
    },
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

describe("Gravitee connector instance form", () => {
  it("offers the complete fixed boundary and a compatible Vault without accepting a token", async () => {
    const user = userEvent.setup();
    render(
      <GraviteeInstanceForm
        instance={null}
        instances={[vault]}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Prazo mínimo (segundos)")).toHaveValue(86_400);
    expect(screen.getByLabelText("Prazo máximo (segundos)")).toHaveValue(7_776_000);
    expect(screen.getByLabelText("Campo do token")).toHaveValue("access_token");
    await user.click(screen.getByRole("combobox", { name: "Instância Vault" }));
    expect(await screen.findByRole("option", { name: "secrets · acme" })).toBeInTheDocument();
    expect(screen.queryByLabelText(/token do gravitee/i)).not.toBeInTheDocument();
  });

  it("preserves the fixed gateway and Vault boundary while editing", async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => undefined);
    render(
      <GraviteeInstanceForm
        instance={gravitee}
        instances={[vault]}
        onClose={vi.fn()}
        onSave={save}
      />,
    );

    expect(screen.getByLabelText("Nome da instância")).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Escopo da instância" })).toBeDisabled();
    expect(screen.queryByLabelText(/token do gravitee/i)).not.toBeInTheDocument();
    await user.clear(screen.getByLabelText("Ambiente"));
    await user.type(screen.getByLabelText("Ambiente"), "env-next");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save).toHaveBeenCalledWith({
      name: "apim",
      body: {
        connector: "gravitee",
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        gravitee: {
          ...gravitee.gravitee,
          environment: "env-next",
        },
      },
    });
  });

  it("explains when the selected Vault does not own the credential path", async () => {
    const user = userEvent.setup();
    render(
      <GraviteeInstanceForm
        instance={{
          ...gravitee,
          gravitee: {
            ...gravitee.gravitee!,
            credentialSource: {
              ...gravitee.gravitee!.credentialSource,
              path: "outside/token",
            },
          },
        }}
        instances={[vault]}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Salvar" }));
    expect(await screen.findByText(/fora dos prefixos permitidos/i)).toBeInTheDocument();
  });
});
