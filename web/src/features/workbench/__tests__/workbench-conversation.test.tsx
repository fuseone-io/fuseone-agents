import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import {
  draftOf,
  openAt,
  polyfillPointer,
  serve,
  type Call,
  type Routes,
} from "@/features/workbench/__tests__/workbench-fixtures";

/*
The workbench's conversation, walked as a person walks it (NT-018 phase 3).

The model only proposes: what it proposes is a card the person accepts or
rejects, and only accepting changes the draft. A turn is sent once under its
key, whatever the network does; a conversation closed for the day says why
before anybody writes into it.
*/

const DRAFT = "/agents/triagem/draft";
const TURNS = "/agents/triagem/draft/turns";
const PROPOSALS = "/agents/triagem/draft/proposals";
const posts = (calls: Call[], path: string) => calls.filter((c) => c.method === "POST" && c.path === path);

const turn = { turnId: "t-1", key: "k", state: "done", erased: false, pasted: false, readRevision: '"e1"', reservedMicros: 90_000, costMicros: 4_500, createdAt: "2026-11-17T12:00:00Z" };
const blockProposal = { proposalId: "p-1", turnId: "t-1", kind: "replace_block", valid: true, readRevision: '"e1"', createdAt: "2026-11-17T12:00:01Z",
  operation: { kind: "replace_block", blockId: "b1", text: "Triar, responder e encerrar.", locale: "pt-BR" } };

function world(over: Routes = {}, conversation = "open") {
  // The thread holds what a send answered, as the server's would.
  const thread: unknown[] = [];
  const send = over[`POST ${TURNS}`];
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${TURNS}`]: () => ({ body: { items: thread, conversation } }),
    ...(send && {
      [`POST ${TURNS}`]: async (call, calls) => {
        const reply = await send(call, calls);
        if (reply !== "hang" && (reply.status ?? 200) === 200) thread.push(reply.body);
        return reply;
      },
    }),
    [`GET ${PROPOSALS}`]: () => ({ body: { items: [] } }),
    ...Object.fromEntries(Object.entries(over).filter(([route]) => route !== `POST ${TURNS}`)),
  });
}

/** The message box, once the conversation is known to be open. */
async function messageBox() {
  const box = await screen.findByLabelText("Mensagem para o assistente");
  await waitFor(() => expect(box).toBeEnabled());
  return box;
}

async function say(text: string) {
  await userEvent.type(await messageBox(), text);
  await userEvent.click(screen.getByRole("button", { name: "Enviar" }));
}

describe("the workbench conversation", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("says why it is closed before anybody writes into it", async () => {
    world({}, "spent");
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("O teto de autoria de hoje foi atingido")).toBeInTheDocument();
    expect(screen.getByLabelText("Mensagem para o assistente")).toBeDisabled();
  });

  it("explains what it is for while it holds no turn", async () => {
    world();
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("Ainda sem conversa")).toBeInTheDocument();
  });

  it("sends a message once under its key, with the locale, and shows the answer", async () => {
    const calls = world({
      [`POST ${TURNS}`]: (call) => ({ body: { ...turn, key: (call.body as { key: string }).key, message: "Ajude.", reply: "Sugiro encerrar." } }),
    });
    openAt("/agents/triagem/workbench");
    await say("Ajude.");
    expect(await screen.findByText("Sugiro encerrar.")).toBeInTheDocument();
    const sent = posts(calls, TURNS).map((c) => c.body as { key: string });
    expect(sent).toEqual([expect.objectContaining({ message: "Ajude.", locale: "pt-BR", pasted: false })]);
    expect(sent[0]?.key).toMatch(/.{8,}/);
  });

  it("sends the same key again when the first send never got an answer", async () => {
    let tries = 0;
    const calls = world({ [`POST ${TURNS}`]: () => (++tries === 1 ? { status: 503 } : { body: { ...turn, message: "Oi" } }) });
    openAt("/agents/triagem/workbench");
    await say("Oi");
    await userEvent.click(await screen.findByRole("button", { name: "Enviar" }));
    await screen.findAllByText("Oi");
    const [first, second] = posts(calls, TURNS).map((c) => (c.body as { key: string }).key);
    expect(second).toBe(first);
  });

  it("marks a message that was pasted", async () => {
    const calls = world({ [`POST ${TURNS}`]: () => ({ body: { ...turn, message: "colado", pasted: true } }) });
    openAt("/agents/triagem/workbench");
    await userEvent.click(await messageBox());
    await userEvent.paste("colado");
    await userEvent.click(screen.getByRole("button", { name: "Enviar" }));
    expect(await screen.findByText("Colado")).toBeInTheDocument();
    expect(posts(calls, TURNS)[0]?.body).toMatchObject({ pasted: true });
  });

  it("shows a block proposal as its difference, and applies it only when accepted", async () => {
    const accepted = draftOf({ instructions: "Objetivo\nTriar, responder e encerrar." });
    const calls = world({
      [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, message: "Melhore.", reply: "Proponho." }], conversation: "open" } }),
      [`GET ${PROPOSALS}`]: () => ({ body: { items: [blockProposal] } }),
      [`POST ${PROPOSALS}/p-1/accept`]: () => ({ body: accepted, etag: '"e2"' }),
    });
    openAt("/agents/triagem/workbench");
    const card = await screen.findByRole("article", { name: "Reescrever um bloco" });
    expect(within(card).getByText(/encerrar/)).toBeInTheDocument();
    await userEvent.click(within(card).getByRole("button", { name: "Aceitar" }));
    expect(await screen.findByText("Proposta aplicada ao rascunho")).toBeInTheDocument();
    expect(posts(calls, `${PROPOSALS}/p-1/accept`)).toEqual([expect.objectContaining({ body: undefined })]);
  });

  it("says a proposal was made over an older draft when accepting it is refused", async () => {
    world({
      [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, message: "Melhore." }], conversation: "open" } }),
      [`GET ${PROPOSALS}`]: () => ({ body: { items: [blockProposal] } }),
      [`POST ${PROPOSALS}/p-1/accept`]: () => ({ status: 412, etag: '"e9"' }),
    });
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: "Aceitar" }));
    expect(await screen.findByText("Esta proposta foi feita sobre uma versão anterior do rascunho")).toBeInTheDocument();
  });

  it("says a turn failed, and what it cost", async () => {
    world({ [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, state: "failed", message: "Oi" }], conversation: "open" } }) });
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("O assistente não respondeu")).toBeInTheDocument();
    expect(screen.getByText(/0,0045/)).toBeInTheDocument();
  });

  it("erases the conversation only once confirmed", async () => {
    const calls = world({
      [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, message: "Oi" }], conversation: "open" } }),
      [`DELETE /agents/triagem/draft/thread`]: () => ({ status: 204 }),
    });
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: "Apagar conversa" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Apagar" }));
    expect(await screen.findByText("Conversa apagada")).toBeInTheDocument();
    expect(calls.some((c) => c.method === "DELETE" && c.path === "/agents/triagem/draft/thread")).toBe(true);
  });

  it("takes an accepted field proposal into the panel, under what the person has not saved", async () => {
    const tool = { ...blockProposal, kind: "add_tool", operation: { kind: "add_tool", tool: "crm.refund" } };
    world({
      [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, message: "Reembolso." }], conversation: "open" } }),
      [`GET ${PROPOSALS}`]: () => ({ body: { items: [tool] } }),
      [`POST ${PROPOSALS}/p-1/accept`]: () => ({ body: draftOf({ tools: ["crm.lookup", "crm.refund"] }), etag: '"e2"' }),
      "GET /admin/tools": () => ({ body: { items: ["crm.lookup", "crm.refund"].map((toolId) => ({ toolId, server: "crm", effect: "read", untrusted: false })) } }),
    });
    openAt("/agents/triagem/workbench");
    const name = await screen.findByLabelText(/^Nome/);
    await userEvent.clear(name);
    await userEvent.type(name, "Triagem nova");
    await userEvent.click(await screen.findByRole("button", { name: "Aceitar" }));
    await screen.findByText("Proposta aplicada ao rascunho");
    // The field, not only the card: granted now, and the unsaved name kept.
    await waitFor(() => expect(screen.getByRole("checkbox", { name: /crm\.refund/ })).toBeChecked());
    expect(screen.getByLabelText(/^Nome/)).toHaveValue("Triagem nova");
  });

  it("shows a proposal only a person may decide as refused, with nothing to accept", async () => {
    const identity = { ...blockProposal, kind: "set_identity", valid: false, refusal: "drafts: human only",
      operation: { kind: "set_identity", name: "Outro", company: "acme", area: "cx" } };
    world({
      [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, message: "Renomeie." }], conversation: "open" } }),
      [`GET ${PROPOSALS}`]: () => ({ body: { items: [identity] } }),
    });
    openAt("/agents/triagem/workbench");
    const card = await screen.findByRole("article", { name: "Mudar nome ou escopo" });
    expect(within(card).getByText(/decisões da própria pessoa/)).toBeInTheDocument();
    expect(within(card).queryByRole("button", { name: "Aceitar" })).not.toBeInTheDocument();
  });

  it.each([
    ["set_budget", { budget: { micros: 250_000, steps: 30 } }, /"steps": 30/],
    ["set_triggers", { triggers: [{ type: "cron", schedule: "0 9 * * 1-5" }] }, /0 9 \* \* 1-5/],
    ["set_memory_learning", { memoryLearning: { mode: "off" } }, /"mode": "off"/],
    ["set_approval_delivery", { approvals: { direct: true } }, /"direct": true/],
    ["set_emits", { emits: [{ event: "ticket.triaged" }] }, /ticket\.triaged/],
    ["edit_step", { stepName: "Buscar", step: { name: "Buscar o cliente" } }, /Buscar o cliente/],
    ["set_fallback", {}, /Sem fallback/],
    ["set_reply_locale", { replyLocale: "" }, /Sem fixação/],
    ["set_cache_prefix_ttl", { cachePrefixTtl: "" }, /Padrão do provedor/],
  ])("shows what a %s proposal sets before it can be accepted", async (kind, fields, shown) => {
    const proposal = { ...blockProposal, kind, operation: { kind, ...fields } };
    world({
      [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, message: "Ajuste." }], conversation: "open" } }),
      [`GET ${PROPOSALS}`]: () => ({ body: { items: [proposal] } }),
    });
    openAt("/agents/triagem/workbench");
    const card = await screen.findByRole("article");
    // The card's whole text: a JSON value is coloured in pieces.
    expect(card).toHaveTextContent(shown);
    expect(within(card).getByRole("button", { name: "Aceitar" })).toBeInTheDocument();
  });

  it("offers nothing to accept when it cannot show what a proposal sets", async () => {
    const proposal = { ...blockProposal, kind: "set_budget", operation: { kind: "set_budget" } };
    world({
      [`GET ${TURNS}`]: () => ({ body: { items: [{ ...turn, message: "Ajuste." }], conversation: "open" } }),
      [`GET ${PROPOSALS}`]: () => ({ body: { items: [proposal] } }),
    });
    openAt("/agents/triagem/workbench");
    const card = await screen.findByRole("article", { name: "Mudar o teto" });
    expect(within(card).getByText(/não é possível mostrar/i)).toBeInTheDocument();
    expect(within(card).queryByRole("button", { name: "Aceitar" })).not.toBeInTheDocument();
  });

});
