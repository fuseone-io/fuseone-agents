import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
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

function stubApi(
  options: {
    can?: string[];
    agents?: unknown[];
    requests?: { method: string; url: string; body?: unknown }[];
  } = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = input instanceof Request ? input.url : String(input);
      const method = input instanceof Request ? input.method : "GET";
      if (options.requests) {
        const text =
          input instanceof Request && method !== "GET"
            ? await input.clone().text()
            : "";
        options.requests.push({
          method,
          url,
          body: text ? JSON.parse(text) : undefined,
        });
      }
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
              ? {
                  items: options.agents ?? [
                    { agentId: "troubleshooting-sre", name: "Troubleshooting SRE" },
                  ],
                }
          : {};
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function renderForm(
  conversation?: ComponentProps<typeof ConversationForm>["conversation"],
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  return render(
    <QueryClientProvider client={client}>
      <ConversationForm
        channel="cora-slack"
        conversation={conversation}
        onClose={() => {}}
      />
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

  it("only offers agents that declared the Conversation trigger for watched messages", async () => {
    stubApi({
      agents: [
        { agentId: "internal-only", name: "Internal only", triggers: [] },
        {
          agentId: "troubleshooting-sre",
          name: "Troubleshooting SRE",
          triggers: [{ type: "channel" }],
        },
      ],
    });
    const user = userEvent.setup();
    renderForm({
      id: "C-alerts",
      label: "#alerts",
      scope: { company: "cora", area: "devops" },
      mode: "watch",
      sources: ["B0123ALERT"],
      agent: "",
      runAs: "usr_opsbot",
      wants: ["parked"],
      enabled: true,
    });

    await user.click(await screen.findByRole("combobox", { name: /Iniciar agente/ }));

    expect(
      await screen.findByRole("option", { name: "Troubleshooting SRE" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Internal only" }),
    ).not.toBeInTheDocument();
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

  it("edits an existing conversation instead of creating a second one", async () => {
    renderForm({
      id: "C-alerts",
      label: "#alerts",
      scope: { company: "cora", area: "devops" },
      mode: "watch",
      sources: ["B0123ALERT", "A0123APP"],
      agent: "troubleshooting-sre",
      runAs: "usr_opsbot",
      wants: ["parked", "failed", "finished"],
      enabled: true,
    });

    expect(await screen.findByText("Editar conversa")).toBeInTheDocument();
    const id = screen.getByDisplayValue("C-alerts");
    expect(id).toBeDisabled();
    expect(
      await screen.findByLabelText("Fontes Slack permitidas"),
    ).toHaveValue("B0123ALERT\nA0123APP");
    expect(
      screen.getByText(/Para apontar outro canal Slack/),
    ).toBeInTheDocument();
  });

  it("sends the mention thread context choice when a conversation is saved", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ requests });
    const user = userEvent.setup();
    renderForm({
      id: "C-alerts",
      label: "#alerts",
      scope: { company: "cora", area: "devops" },
      mode: "mentions",
      threadContext: false,
      wants: ["parked", "failed"],
      enabled: true,
    });

    expect(await screen.findByText("Editar conversa")).toBeInTheDocument();
    await user.click(screen.getByText("Incluir contexto da thread"));
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() =>
      expect(requests.some((r) => r.method === "PUT")).toBe(true),
    );
    const put = requests.find((r) => r.method === "PUT");
    expect(put?.body).toMatchObject({
      company: "cora",
      area: "devops",
      mode: "mentions",
      threadContext: true,
    });
  });

  it("can keep mentions and watched messages enabled together", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ requests });
    const user = userEvent.setup();
    renderForm({
      id: "C-alerts",
      label: "#alerts",
      scope: { company: "cora", area: "devops" },
      mode: "both",
      threadContext: false,
      sources: ["B0123ALERT", "A0123APP"],
      agent: "troubleshooting-sre",
      runAs: "usr_opsbot",
      wants: ["parked", "failed"],
      enabled: true,
    });

    expect(await screen.findByText("Iniciar agente")).toBeInTheDocument();
    expect(screen.getByText("Fontes Slack permitidas")).toBeInTheDocument();
    await user.click(screen.getByText("Incluir contexto da thread"));
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() =>
      expect(requests.some((r) => r.method === "PUT")).toBe(true),
    );
    const put = requests.find((r) => r.method === "PUT");
    expect(put?.body).toMatchObject({
      company: "cora",
      area: "devops",
      mode: "both",
      threadContext: true,
      sources: ["B0123ALERT", "A0123APP"],
      agent: "troubleshooting-sre",
      runAs: "usr_opsbot",
    });
  });
});
