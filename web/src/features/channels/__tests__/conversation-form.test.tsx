import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
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
          ? {
              items: [
                { company: "acme", area: "devops", label: "Devops" },
                { company: "acme", area: "ops", label: "Ops" },
              ],
            }
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
        channel="acme-slack"
        conversation={conversation}
        onClose={() => {}}
      />
    </QueryClientProvider>,
  );
}

const sre = {
  agentId: "troubleshooting-sre",
  name: "Troubleshooting SRE",
  triggers: [{ type: "channel" }],
};

const mentionsConversation = {
  id: "C-alerts",
  label: "#alerts",
  scope: { company: "acme", area: "devops" },
  mode: "mentions" as const,
  wants: ["parked"],
  enabled: true,
};


function saved(requests: { method: string; body?: unknown }[]) {
  return requests.find((one) => one.method === "PUT")?.body;
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

  /*
   * Sending the card privately as well is a choice about who sees a run's
   * facts, so it is offered where the events are chosen and only where parked
   * runs are among them. A conversation that is never told a run stopped
   * cannot tell anybody privately either, and offering the option there would
   * be a switch that does nothing.
   */
  it("offers private approvals only where parked runs are announced", async () => {
    const user = userEvent.setup();
    renderForm({
      ...mentionsConversation,
      wants: ["parked"],
    });

    expect(
      await screen.findByText("Também avisar em particular quem pode decidir"),
    ).toBeInTheDocument();

    await user.click(screen.getByText("Esperando uma decisão"));

    expect(
      screen.queryByText("Também avisar em particular quem pode decidir"),
    ).not.toBeInTheDocument();
  });

  /*
   * Turning off the event takes the private card with it, on the way out and
   * not only on the way back. The field disappears when parked is unchecked
   * and its value does not, so a request could say one thing while the server
   * stores another — and the console would show it off after the next read,
   * with nothing saying why.
   */
  it("drops the private approval choice when parked stops being announced", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ requests });
    const user = userEvent.setup();
    renderForm({
      ...mentionsConversation,
      wants: ["parked", "failed"],
      directApprovals: true,
    });

    await user.click(await screen.findByText("Esperando uma decisão"));
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(saved(requests)).toBeDefined());
    expect(saved(requests)).toMatchObject({ directApprovals: false });
  });

  it("sends the private approval choice when a conversation is saved", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ requests });
    const user = userEvent.setup();
    renderForm({ ...mentionsConversation, wants: ["parked"] });

    await user.click(
      await screen.findByText("Também avisar em particular quem pode decidir"),
    );
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(saved(requests)).toBeDefined());
    expect(saved(requests)).toMatchObject({ directApprovals: true });
  });

  /*
   * A conversation that only reports offers nothing about starting runs.
   *
   * The predicate deciding that was a denylist — anything that is not "watch"
   * takes mentions — so a mode whose whole purpose is to start nothing would
   * have shown the mention fields and offered to configure an inbound path the
   * server refuses.
   */
  it("offers nothing about starting runs for a conversation that only reports", async () => {
    renderForm({ ...mentionsConversation, mode: "announce" });

    expect(await screen.findByText(/O que avisar/)).toBeInTheDocument();
    expect(screen.queryByText("Incluir contexto da thread")).not.toBeInTheDocument();
    expect(screen.queryByText("Fontes Slack permitidas")).not.toBeInTheDocument();
    expect(screen.queryByText("Rodar como")).not.toBeInTheDocument();
  });

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
    // The agent belongs to the conversation whatever starts its runs; the
    // principal and the sources belong to watched messages alone.
    expect(screen.getByText("Agente desta conversa")).toBeInTheDocument();
    expect(screen.queryByText("Fontes Slack permitidas")).not.toBeInTheDocument();
    expect(screen.queryByText("Rodar como")).not.toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: /O que inicia runs/ }));
    await user.click(
      await screen.findByRole("option", {
        name: "Observar mensagens selecionadas",
      }),
    );

    expect(screen.getByText("Agente desta conversa")).toBeInTheDocument();
    expect(screen.getByText("Fontes Slack permitidas")).toBeInTheDocument();
    expect(screen.getByText("Rodar como")).toBeInTheDocument();
  });

  /*
   * A conversation that only takes mentions may still say which agent it is
   * for, and that is what lets somebody mention the bot without naming one.
   * Optional here and only here: a watched message carries no text to read a
   * name out of.
   */
  it("binds an agent to a conversation that only takes mentions", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ requests, agents: [sre] });
    const user = userEvent.setup();
    renderForm(mentionsConversation);

    const picker = await screen.findByRole("combobox", {
      name: /Agente desta conversa/,
    });
    expect(within(picker).getByText(/Nenhum/)).toBeInTheDocument();
    await user.click(picker);
    await user.click(await screen.findByRole("option", { name: sre.name }));
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(saved(requests)).toBeDefined());
    expect(saved(requests)).toMatchObject({
      mode: "mentions",
      agent: sre.agentId,
    });
    // The principal is the watched half and must not travel with a mention.
    expect(saved(requests)).not.toHaveProperty("runAs");
  });

  /*
   * An agent is only startable in the scope it is published in, so one chosen
   * for another scope is a configuration the server refuses — and the person
   * finds out on save, having been shown a name the whole time.
   */
  it("clears the agent when the conversation moves to another scope", async () => {
    stubApi({ agents: [sre] });
    const user = userEvent.setup();
    renderForm({ ...mentionsConversation, agent: sre.agentId });

    const picker = await screen.findByRole("combobox", {
      name: /Agente desta conversa/,
    });
    await waitFor(() =>
      expect(within(picker).getByText(sre.name)).toBeInTheDocument(),
    );

    await user.click(screen.getByRole("combobox", { name: "Contexto" }));
    await user.click(await screen.findByRole("option", { name: "Ops" }));

    expect(within(picker).getByText(/Nenhum/)).toBeInTheDocument();
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
      scope: { company: "acme", area: "devops" },
      mode: "watch",
      sources: ["B0123ALERT"],
      agent: "",
      runAs: "usr_opsbot",
      wants: ["parked"],
      enabled: true,
    });

    await user.click(await screen.findByRole("combobox", { name: /Agente desta conversa/ }));

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
      scope: { company: "acme", area: "devops" },
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
      scope: { company: "acme", area: "devops" },
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
      company: "acme",
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
      scope: { company: "acme", area: "devops" },
      mode: "both",
      threadContext: false,
      sources: ["B0123ALERT", "A0123APP"],
      agent: "troubleshooting-sre",
      runAs: "usr_opsbot",
      wants: ["parked", "failed"],
      enabled: true,
    });

    expect(await screen.findByText("Agente desta conversa")).toBeInTheDocument();
    expect(screen.getByText("Fontes Slack permitidas")).toBeInTheDocument();
    await user.click(screen.getByText("Incluir contexto da thread"));
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() =>
      expect(requests.some((r) => r.method === "PUT")).toBe(true),
    );
    const put = requests.find((r) => r.method === "PUT");
    expect(put?.body).toMatchObject({
      company: "acme",
      area: "devops",
      mode: "both",
      threadContext: true,
      sources: ["B0123ALERT", "A0123APP"],
      agent: "troubleshooting-sre",
      runAs: "usr_opsbot",
    });
  });
  /*
   * A room that hears every company is the authority above them all.
   *
   * The server refuses it from anybody else, so offering the option to a
   * curator of one company would be a control that answers 403 — and the
   * console already knows: "company:write" is announced only when it was
   * granted at the installation.
   */
  it("offers the whole installation only to whoever governs it", async () => {
    const user = userEvent.setup();
    stubApi({ can: ["provider:write"] });
    renderForm();

    await user.click(await screen.findByRole("combobox", { name: "Contexto" }));

    expect(
      screen.queryByRole("option", { name: "A instalação inteira" }),
    ).not.toBeInTheDocument();
    expect(await screen.findByRole("option", { name: "Devops" })).toBeInTheDocument();
  });

  /*
   * Nothing inbound is offered for the installation, and nothing inbound is
   * asked of the server either.
   *
   * Listing agents there is a read almost nobody may do, so leaving the query
   * on would paint an error over a field that is not even shown.
   */
  it("asks nothing about starting runs for the whole installation", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ can: ["company:write"], requests });
    const user = userEvent.setup();
    renderForm();

    await user.click(await screen.findByRole("combobox", { name: "Contexto" }));
    await user.click(
      await screen.findByRole("option", { name: "A instalação inteira" }),
    );

    await waitFor(() =>
      expect(
        screen.queryByRole("combobox", { name: /O que inicia runs/ }),
      ).not.toBeInTheDocument(),
    );
    expect(screen.queryByText("Agente desta conversa")).not.toBeInTheDocument();
    expect(screen.queryByText("Incluir contexto da thread")).not.toBeInTheDocument();
    expect(
      requests.filter((one) => one.url.includes("/agents?")),
    ).toHaveLength(0);
  });

  /*
   * What is saved says what the platform will do. The mode field is hidden
   * rather than set, so a request that carried its old value would store a
   * conversation claiming an inbound path the server strips — and the console
   * would disagree with itself on the next read.
   */
  it("saves a conversation for the whole installation as one that only reports", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ can: ["company:write"], requests });
    const user = userEvent.setup();
    renderForm({ ...mentionsConversation, threadContext: true });

    await user.click(await screen.findByRole("combobox", { name: "Contexto" }));
    await user.click(
      await screen.findByRole("option", { name: "A instalação inteira" }),
    );
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(saved(requests)).toBeDefined());
    expect(saved(requests)).toMatchObject({
      company: "*",
      mode: "announce",
      threadContext: false,
    });
    expect(saved(requests)).not.toHaveProperty("area");
    expect(saved(requests)).not.toHaveProperty("agent");
  });
  /*
   * "Only reports" is a mode any conversation may take, and taking it drops
   * the agent — a binding on a conversation that starts nothing is a field
   * stored and never read. The mode picker stays, because a room set to report
   * has to be able to become one that answers again.
   */
  it("drops the bound agent when a conversation is set to only report", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ requests, agents: [sre] });
    const user = userEvent.setup();
    renderForm({ ...mentionsConversation, agent: "troubleshooting-sre" });

    await user.click(
      await screen.findByRole("combobox", { name: /O que inicia runs/ }),
    );
    await user.click(await screen.findByRole("option", { name: "Só relata" }));

    expect(screen.queryByText("Agente desta conversa")).not.toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: /O que inicia runs/ }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(saved(requests)).toBeDefined());
    expect(saved(requests)).toMatchObject({ mode: "announce" });
    expect(saved(requests)).not.toHaveProperty("agent");
  });
  /*
   * A half-written watch rule must not trap somebody in the form.
   *
   * The mode field is hidden for the installation and the request says
   * "announce", but the schema went on validating the mode still sitting in
   * the form: an incomplete watch draft failed on sources, an agent and a
   * principal — three fields the screen was no longer showing — and the save
   * did nothing, with nothing to fix and nowhere to fix it.
   */
  it("saves an unfinished watch draft once the installation is picked", async () => {
    const requests: { method: string; url: string; body?: unknown }[] = [];
    stubApi({ can: ["company:write"], requests });
    const user = userEvent.setup();
    renderForm({ ...mentionsConversation, mode: "watch" });

    await user.click(await screen.findByRole("combobox", { name: "Contexto" }));
    await user.click(
      await screen.findByRole("option", { name: "A instalação inteira" }),
    );
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => expect(saved(requests)).toBeDefined());
    expect(saved(requests)).toMatchObject({ company: "*", mode: "announce" });
  });
  /*
   * A conversation this console cannot draw is not offered for editing.
   *
   * Its mode came from a newer version, and the platform keeps it: a mode
   * nothing here can name starts nothing. The form would fill every field with
   * this console's idea of the nearest value and write that back on save,
   * which is how a room that started nothing becomes one anybody can start
   * runs from by typing in it.
   */
  it("refuses to edit a conversation whose mode came from a newer version", async () => {
    renderForm({ ...mentionsConversation, mode: "a-future-mode" });

    expect(await screen.findByText(/a-future-mode/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Salvar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Contexto" })).not.toBeInTheDocument();
  });
});
