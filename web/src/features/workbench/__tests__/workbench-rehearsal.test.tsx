import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve, type Call, type Routes } from "@/features/workbench/__tests__/workbench-fixtures";
import type { Run, Step } from "@/lib/api/client";

/*
The workbench's rehearsal tab (NT-018 phase 4, release 4c). A candidate is rehearsed on a
case the author writes; the run reaches no tool, so the author answers each call for the
world and decides each action the Gate holds. The tab polls while the rehearsal runs and
stops where it waits for a person, and the author can end it wherever it stands.
*/

const DRAFT = "/agents/triagem/draft";
const RUN = "/runs/run-reh";
const seal = { version: "v2abc", sealId: "s-1", company: "acme", area: "cx", sealedBy: "usr_ana", sealedAt: "2026-11-19T12:00:00Z" };

const run = (over: Partial<Run> = {}): Run => ({
  runId: "run-reh", scope: { company: "acme", area: "cx" }, agentId: "triagem", versionId: "v2abc",
  phase: "running", seq: 1, startedAt: "2026-11-20T12:00:00Z", cost: { micros: 0 }, ...over,
});
const step = (seq: number, kind: Step["kind"], payload: Record<string, unknown> = {}): Step => ({
  seq, kind, at: "2026-11-20T12:00:00Z", hash: `h${seq}`, payload,
});
const waiting = [step(1, "run_started"), step(2, "gate_decided"), step(3, "tool_called", { tool: "crm.lookup" }), step(4, "world_awaited", { tool: "crm.lookup", call_seq: 3 })];

function world(over: Routes = {}) {
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
    ["GET /agents/triagem/candidates"]: () => ({ body: { items: [seal] } }),
    [`GET ${RUN}/steps`]: () => ({ body: { items: waiting } }),
    [`GET ${RUN}/steps/3/content`]: () => ({ body: { seq: 3, digest: "d", content: '{"id":"c-1"}' } }),
    [`GET ${RUN}/steps/7/content`]: () => ({ body: { seq: 7, digest: "d", content: '{"note":"refund"}' } }),
    ...over,
  });
}

async function openTab(at = "/agents/triagem/workbench") {
  openAt(at);
  await userEvent.click(await screen.findByRole("tab", { name: "Ensaio" }));
}
const atRehearsal = "/agents/triagem/workbench?rehearsal=run-reh";
const sent = (calls: Call[], path: string) => calls.find((c) => c.method === "POST" && c.path === path);
const posts = (calls: Call[], path: string) => calls.filter((c) => c.method === "POST" && c.path === path);

describe("the workbench's rehearsal", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("asks for a candidate first while there is none to rehearse", async () => {
    world({ ["GET /agents/triagem/candidates"]: () => ({ status: 404 }) });
    await openTab();
    expect(await screen.findByText("Sele uma candidata primeiro")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Abrir ensaio" })).not.toBeInTheDocument();
  });

  it("opens a rehearsal of the chosen candidate on the case written, under a key of its own", async () => {
    const calls = world({ ["POST /agents/triagem/rehearsals"]: () => ({ status: 201, body: run() }), [`GET ${RUN}`]: () => ({ body: run() }) });
    await openTab();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("combobox", { name: "Candidata" }));
    await user.click(await screen.findByRole("option", { name: /v2abc/ }));
    await user.type(screen.getByRole("textbox", { name: "Caso" }), "A cliente pede reembolso.");
    await user.click(screen.getByRole("button", { name: "Abrir ensaio" }));
    const opened = await waitFor(() => sent(calls, "/agents/triagem/rehearsals")!);
    expect(opened.body).toEqual({ sealId: "s-1", case: "A cliente pede reembolso." });
    expect(opened.headers.get("Idempotency-Key")).toMatch(/.{8,}/);
    expect(await screen.findByText("Em execução")).toBeInTheDocument();
  });

  it("asks again under the same key after a lost answer, and under another for another case", async () => {
    const calls = world({
      ["POST /agents/triagem/rehearsals"]: (_, all) => (posts(all, "/agents/triagem/rehearsals").length < 3 ? { status: 503 } : { status: 201, body: run() }),
      [`GET ${RUN}`]: () => ({ body: run() }),
    });
    await openTab();
    const user = userEvent.setup();
    const ask = async (text: string, n: number) => {
      await user.clear(screen.getByRole("textbox", { name: "Caso" }));
      await user.type(screen.getByRole("textbox", { name: "Caso" }), text);
      await user.click(screen.getByRole("button", { name: "Abrir ensaio" }));
      await waitFor(() => expect(posts(calls, "/agents/triagem/rehearsals")).toHaveLength(n));
    };
    await screen.findByRole("button", { name: "Abrir ensaio" });
    await ask("Reembolso.", 1);
    await ask("Troca.", 2);
    await ask("Reembolso.", 3);
    const [first, other, again] = posts(calls, "/agents/triagem/rehearsals").map((c) => c.headers.get("Idempotency-Key"));
    expect(again).toBe(first);
    expect(other).not.toBe(first);
  });

  it("opens a new rehearsal of the same case under a new key once the first one exists", async () => {
    const calls = world({ ["POST /agents/triagem/rehearsals"]: () => ({ status: 201, body: run({ phase: "finished" }) }), [`GET ${RUN}`]: () => ({ body: run({ phase: "finished" }) }) });
    await openTab();
    const user = userEvent.setup();
    for (const n of [1, 2]) {
      await user.type(await screen.findByRole("textbox", { name: "Caso" }), "Reembolso.");
      await user.click(screen.getByRole("button", { name: "Abrir ensaio" }));
      await user.click(await screen.findByRole("button", { name: "Novo ensaio" }));
      expect(posts(calls, "/agents/triagem/rehearsals")).toHaveLength(n);
    }
    const [first, second] = posts(calls, "/agents/triagem/rehearsals").map((c) => c.headers.get("Idempotency-Key"));
    expect(second).not.toBe(first);
  });

  it("refuses an empty case before asking the server", async () => {
    const calls = world();
    await openTab();
    await userEvent.click(await screen.findByRole("button", { name: "Abrir ensaio" }));
    expect(await screen.findByText("Escreva o caso a ensaiar.")).toBeInTheDocument();
    expect(sent(calls, "/agents/triagem/rehearsals")).toBeUndefined();
  });

  it("shows the call the rehearsal waits on and sends the author's answer for that wait", async () => {
    const calls = world({ [`GET ${RUN}`]: () => ({ body: run({ phase: "awaiting_world", seq: 4 }) }), [`POST ${RUN}/world`]: () => ({ body: run() }) });
    await openTab(atRehearsal);
    const card = await screen.findByRole("region", { name: "A execução espera a resposta do mundo" });
    expect(within(card).getByText("crm.lookup")).toBeInTheDocument();
    expect(await within(card).findByText(/c-1/)).toBeInTheDocument();
    await userEvent.type(within(card).getByRole("textbox", { name: "Resposta" }), '{{"plan":"gold"}');
    await userEvent.click(within(card).getByRole("button", { name: "Responder" }));
    await waitFor(() => expect(sent(calls, `${RUN}/world`)?.body).toEqual({ atSeq: 4, answer: '{"plan":"gold"}' }));
  });

  it("decides the action the Gate held, as a rehearsal decision", async () => {
    const pendingApproval = { runId: "run-reh", tool: "crm.note", atSeq: 7, requestedAt: "2026-11-20T12:00:00Z", reason: "writes require approval by default" };
    const calls = world({ [`GET ${RUN}`]: () => ({ body: run({ phase: "awaiting_approval", seq: 7, pendingApproval }) }), [`POST ${RUN}/rehearsal-decisions`]: () => ({ body: run() }) });
    await openTab(atRehearsal);
    const card = await screen.findByRole("region", { name: "O Gate segurou uma ação" });
    expect(within(card).getByText("crm.note")).toBeInTheDocument();
    expect(await within(card).findByText(/refund/)).toBeInTheDocument();
    await userEvent.click(within(card).getByRole("button", { name: "Recusar" }));
    await waitFor(() => expect(sent(calls, `${RUN}/rehearsal-decisions`)?.body).toEqual({ atSeq: 7, approved: false }));
    expect(sent(calls, `${RUN}/approvals`)).toBeUndefined();
  });

  it("polls while the rehearsal runs and stops once it waits for the world", async () => {
    let reads = 0;
    world({ [`GET ${RUN}`]: () => ({ body: run(++reads < 2 ? {} : { phase: "awaiting_world", seq: 4 }) }) });
    await openTab(atRehearsal);
    expect(await screen.findByRole("region", { name: "A execução espera a resposta do mundo" }, { timeout: 3000 })).toBeInTheDocument();
    const settled = reads;
    await new Promise((resolve) => setTimeout(resolve, 1600));
    expect(reads).toBe(settled);
  });

  it("ends the rehearsal after asking, and offers a new one", async () => {
    let ended = false;
    const calls = world({
      [`GET ${RUN}`]: () => ({ body: run(ended ? { phase: "failed" } : { phase: "awaiting_world", seq: 4 }) }),
      [`POST ${RUN}/rehearsal-abandon`]: () => ((ended = true), { body: run({ phase: "failed" }) }),
    });
    await openTab(atRehearsal);
    await userEvent.click(await screen.findByRole("button", { name: "Abandonar ensaio" }));
    await userEvent.click(await screen.findByRole("button", { name: "Abandonar" }));
    await waitFor(() => expect(sent(calls, `${RUN}/rehearsal-abandon`)).toBeDefined());
    expect(await screen.findByRole("button", { name: "Novo ensaio" })).toBeInTheDocument();
  });

  it("says why an answer was refused", async () => {
    world({
      [`GET ${RUN}`]: () => ({ body: run({ phase: "awaiting_world", seq: 4 }) }),
      [`POST ${RUN}/world`]: () => ({ status: 409, body: { type: "fuseone:conflict", title: "Conflict", status: 409, detail: "this wait was answered already" } }),
    });
    await openTab(atRehearsal);
    const card = await screen.findByRole("region", { name: "A execução espera a resposta do mundo" });
    await userEvent.type(within(card).getByRole("textbox", { name: "Resposta" }), "ok");
    await userEvent.click(within(card).getByRole("button", { name: "Responder" }));
    expect(await screen.findByText(/answered already|já foi|conflito/i)).toBeInTheDocument();
  });

  it("says why the rehearsal could not be read, and reads it again when asked", async () => {
    let fail = true;
    world({ [`GET ${RUN}`]: () => (fail ? ((fail = false), { status: 500 }) : { body: run({ phase: "finished" }) }) });
    await openTab(atRehearsal);
    await userEvent.click(await screen.findByRole("button", { name: /tentar de novo/i }));
    expect(await screen.findByText("Concluída")).toBeInTheDocument();
  });

  it("says what erasing the conversation reaches, beside the rehearsal", async () => {
    world();
    await openTab();
    expect(await screen.findByText(/só alcança os ensaios desta conversa no rascunho atual/)).toBeInTheDocument();
  });
});
