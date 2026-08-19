import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { ConversationForm } from "@/features/channels/conversation-form";
import { setLocale } from "@/i18n";

function stubApi(options: { can?: string[] } = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/api/v1/me")
        ? {
            id: "usr_opsbot",
            display: "Ops Bot",
            kind: "local",
            grants: [],
            can: options.can ?? [],
          }
        : url.includes("/available")
        ? { items: [] }
        : url.includes("/admin/scopes")
          ? { items: [{ company: "cora", area: "devops", label: "Devops" }] }
          : url.includes("/admin/people")
            ? {
                items: [
                  { id: "usr_opsbot", display: "Ops Bot" },
                  { id: "usr_admin", display: "Security Admin" },
                ],
              }
            : url.includes("/agents")
              ? { items: [{ agentId: "troubleshooting-sre", name: "Troubleshooting SRE" }] }
          : {};
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function renderForm() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  return render(
    <QueryClientProvider client={client}>
      <ConversationForm channel="cora-slack" onClose={() => {}} />
    </QueryClientProvider>,
  );
}

describe("conversation configuration", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  });

  beforeEach(() => {
    vi.restoreAllMocks();
    setLocale("pt-BR");
    stubApi();
  });
  afterEach(() => vi.unstubAllGlobals());

  it("explains an empty Slack listing instead of leaving a picker with no choices", async () => {
    renderForm();

    expect(
      await screen.findByText(/O Slack não retornou nenhum canal/),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText("C0123ABCDEF")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Tentar de novo/ })).toBeEnabled();
  });

  it("shows the watch rule only when selected", async () => {
    const user = userEvent.setup();
    renderForm();

    expect(await screen.findByText(/O que avisar/)).toBeInTheDocument();
    expect(screen.queryByText("Iniciar agente")).not.toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: /O que inicia runs/ }));
    await user.click(
      await screen.findByRole("option", {
        name: "Observar mensagens selecionadas",
      }),
    );

    expect(screen.getByText("Iniciar agente")).toBeInTheDocument();
    expect(screen.getByText("Fontes Slack permitidas")).toBeInTheDocument();
    expect(screen.getByText("Rodar como")).toBeInTheDocument();
  });

  it("only offers the caller as runAs unless they administer identities", async () => {
    const user = userEvent.setup();
    renderForm();

    await user.click(await screen.findByRole("combobox", { name: /O que inicia runs/ }));
    await user.click(
      await screen.findByRole("option", {
        name: "Observar mensagens selecionadas",
      }),
    );

    await user.click(await screen.findByRole("combobox", { name: /Rodar como/ }));

    expect(await screen.findByRole("option", { name: "Ops Bot" })).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Security Admin" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(/Administradores de identidade podem delegar/),
    ).toBeInTheDocument();
  });

  it("lets identity administrators choose another runAs principal", async () => {
    stubApi({ can: ["identity:write"] });
    const user = userEvent.setup();
    renderForm();

    await user.click(await screen.findByRole("combobox", { name: /O que inicia runs/ }));
    await user.click(
      await screen.findByRole("option", {
        name: "Observar mensagens selecionadas",
      }),
    );

    await user.click(await screen.findByRole("combobox", { name: /Rodar como/ }));

    expect(await screen.findByRole("option", { name: "Ops Bot" })).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: "Security Admin" }),
    ).toBeInTheDocument();
  });
});
