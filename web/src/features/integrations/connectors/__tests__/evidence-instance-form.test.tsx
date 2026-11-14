import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderBare, screen, waitFor, within } from "@testing-library/react";
import type { ReactElement } from "react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectorInstanceDetail } from "@/features/integrations/api";
import { evidenceExamples } from "@/features/integrations/connectors/evidence-examples";
import { EvidenceInstanceForm } from "@/features/integrations/connectors/evidence-instance-form";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";

// The free scope reads the registered companies and areas.
const render = (ui: ReactElement) =>
  renderBare(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {ui}
    </QueryClientProvider>,
  );

const stored: ConnectorInstanceDetail = {
  name: "gateway",
  connector: "evidence",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "cyber",
  hasToken: true,
  evidence: evidenceExamples[0]!.declaration,
};

// This form validates on every keystroke over dozens of fields, so typing is slow on a
// busy machine. A test that timed out kept typing in the background, into the next
// test's fields, and the failure looked like the form doubling letters.
vi.setConfig({ testTimeout: 60_000 });

beforeAll(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.setPointerCapture ??= () => {};
  Element.prototype.releasePointerCapture ??= () => {};
  Element.prototype.scrollIntoView ??= () => {};
});

beforeEach(() => {
  setLocale("pt-BR");
  useActiveScope.setState({ company: "acme", area: "cyber" });
});

function renderForm(instance: ConnectorInstanceDetail | null = null) {
  const onSave = vi.fn().mockResolvedValue(undefined);
  render(
    <EvidenceInstanceForm
      instance={instance}
      onClose={vi.fn()}
      onSave={onSave}
    />,
  );
  return { onSave, user: userEvent.setup() };
}

describe("evidence connector instance form", () => {
  it("creates a source from an example, with the name and token a person gave", async () => {
    const { onSave, user } = renderForm();

    await user.click(
      screen.getByRole("combobox", { name: "Começar de um exemplo" }),
    );
    await user.click(
      screen.getByRole("option", { name: evidenceExampleLabel(0) }),
    );
    await user.type(screen.getByLabelText("Nome da instância"), "gateway");
    await user.type(screen.getByLabelText("Token de leitura"), "read-only");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce());
    const saved = onSave.mock.calls[0]![0];
    expect(saved.name).toBe("gateway");
    expect(saved.body.connector).toBe("evidence");
    expect(saved.body.token).toBe("read-only");
    expect(saved.body.evidence).toEqual(evidenceExamples[0]!.declaration);
  });

  it("keeps the name and token already typed when an example is applied", async () => {
    const { onSave, user } = renderForm();

    await user.type(screen.getByLabelText("Nome da instância"), "gateway");
    await user.type(screen.getByLabelText("Token de leitura"), "read-only");
    await user.click(
      screen.getByRole("combobox", { name: "Começar de um exemplo" }),
    );
    await user.click(
      screen.getByRole("option", { name: evidenceExampleLabel(0) }),
    );
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce());
    const saved = onSave.mock.calls[0]![0];
    expect(saved.name).toBe("gateway");
    expect(saved.body.token).toBe("read-only");
  });

  it("does not save a new source with no token", async () => {
    const { onSave, user } = renderForm();

    await user.click(
      screen.getByRole("combobox", { name: "Começar de um exemplo" }),
    );
    await user.click(
      screen.getByRole("option", { name: evidenceExampleLabel(1) }),
    );
    await user.type(screen.getByLabelText("Nome da instância"), "console");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    expect(
      await screen.findByText("Um connector ativo precisa de token."),
    ).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("keeps the stored token when editing, and sends the stored declaration back", async () => {
    const { onSave, user } = renderForm(stored);

    expect(screen.getByLabelText("Nome da instância")).toBeDisabled();
    expect(screen.getByLabelText("Token de leitura")).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce());
    const saved = onSave.mock.calls[0]![0];
    expect(saved.body.token).toBeUndefined();
    expect(saved.body.evidence).toEqual(stored.evidence);
  });

  it("saves a filter and a parameter added to a query, byte for byte", async () => {
    const { onSave, user } = renderForm(stored);
    const query = within(
      screen.getByRole("heading", { name: "pages_by_user" }).parentElement!
        .parentElement!,
    );

    await user.click(query.getByRole("button", { name: "Adicionar filtro" }));
    await user.click(
      query.getByRole("button", { name: "Adicionar parâmetro" }),
    );
    const names = query.getAllByLabelText("Nome do filtro");
    const values = query.getAllByLabelText("Valor do filtro");
    const parameters = query.getAllByLabelText("Nome do parâmetro");
    const patterns = query.getAllByLabelText("Padrão");
    await user.type(names.at(-1)!, "extra");
    await user.click(values.at(-1)!);
    await user.paste("  a {who} ");
    await user.type(parameters.at(-1)!, "who");
    await user.click(patterns.at(-1)!);
    await user.paste(" x ");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce());
    const saved = onSave.mock.calls[0]![0].body.evidence.queries[1];
    expect(saved.filters.at(-1)).toEqual({
      name: "extra",
      value: "  a {who} ",
    });
    expect(saved.parameters.at(-1)).toEqual({ name: "who", pattern: " x " });
  });

  it("offers as the control only a query that can be one", async () => {
    const { user } = renderForm(stored);

    await user.click(
      screen.getByRole("combobox", { name: "Controle positivo" }),
    );

    const options = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(options).toEqual(["all_pages"]);
  });

  it("fills the form from a pasted declaration", async () => {
    const { user } = renderForm();

    await user.click(screen.getByRole("button", { name: "Colar JSON" }));
    await user.click(screen.getByLabelText("Declaração em JSON"));
    await user.paste(JSON.stringify(evidenceExamples[1]!.declaration));
    await user.click(screen.getByRole("button", { name: "Aplicar JSON" }));

    expect(screen.getByLabelText("Endereço da fonte")).toHaveValue(
      "https://console.example.com",
    );
  });

  it("says so when a pasted declaration cannot be read, and changes nothing", async () => {
    const { user } = renderForm();

    await user.type(
      screen.getByLabelText("Endereço da fonte"),
      "https://kept.example.com",
    );
    await user.click(screen.getByRole("button", { name: "Colar JSON" }));
    await user.click(screen.getByLabelText("Declaração em JSON"));
    await user.paste('{"token":"secret"}');
    await user.click(screen.getByRole("button", { name: "Aplicar JSON" }));

    expect(
      screen.getByText("Este JSON não é uma declaração de fonte válida."),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Endereço da fonte")).toHaveValue(
      "https://kept.example.com",
    );
  });
});

function evidenceExampleLabel(at: number) {
  return at === 0
    ? "Gateway (exemplo Netskope): períodos fixos e offset"
    : "EDR (exemplo SentinelOne): intervalo e cursor";
}
