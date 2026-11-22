import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve, type Call, type Routes } from "@/features/workbench/__tests__/workbench-fixtures";
import type { Run, Step } from "@/lib/api/client";

/*
Cases in the workbench (NT-018 phase 5b). An ended rehearsal is kept as a case with the
world its author answered for, and the Casos tab lists the corpus and removes from it.
Two things are said where they are decided: erasing the conversation does not reach a case
once it is kept (D2), and removing a case writes a new version and erases nothing (D1).
*/

const DRAFT = "/agents/triagem/draft";
const RUN = "/runs/run-reh";
const CASES = "/agents/triagem/regressions";

const run = (over: Partial<Run> = {}): Run => ({
  runId: "run-reh", scope: { company: "acme", area: "cx" }, agentId: "triagem", versionId: "v2abc",
  phase: "finished", seq: 6, startedAt: "2026-11-20T12:00:00Z", cost: { micros: 0 }, ...over,
});
const step = (seq: number, kind: Step["kind"], payload: Record<string, unknown> = {}): Step => ({
  seq, kind, at: "2026-11-20T12:00:00Z", hash: `h${seq}`, payload,
});
const trail = [
  step(1, "run_started"), step(2, "gate_decided", { tool: "crm.lookup" }), step(3, "tool_called", { tool: "crm.lookup" }),
  step(4, "world_awaited", { tool: "crm.lookup", call_seq: 3 }), step(5, "world_supplied"), step(6, "run_finished"),
];
const kept = { id: "case-1", expectations: [{ kind: "never_calls", value: "crm.lookup" }], fromRun: "run-reh", note: "não consulta", createdBy: "usr_ana", createdAt: "2026-11-20T12:00:00Z" };

function world(over: Routes = {}) {
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
    [`GET ${RUN}`]: () => ({ body: run() }),
    [`GET ${RUN}/steps`]: () => ({ body: { items: trail } }),
    ...over,
  });
}

async function openTab(name: string, at = "/agents/triagem/workbench") {
  openAt(at);
  await userEvent.click(await screen.findByRole("tab", { name }));
}
const atRehearsal = "/agents/triagem/workbench?rehearsal=run-reh";
const sent = (calls: Call[], method: string, path: string) => calls.find((c) => c.method === method && c.path === path);

async function keepAsCase() {
  await openTab("Ensaio", atRehearsal);
  await userEvent.click(await screen.findByRole("button", { name: "Guardar como caso" }));
  return screen.findByRole("dialog", { name: "Guardar este ensaio como caso" });
}

describe("cases in the workbench", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("keeps an ended rehearsal as a case, checking what the author marked", async () => {
    const calls = world({ [`POST ${CASES}`]: () => ({ status: 201, body: kept }) });
    const dialog = await keepAsCase();
    await userEvent.click(within(dialog).getByRole("checkbox", { name: /nunca deveria chamar/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Guardar caso" }));
    await waitFor(() =>
      expect(sent(calls, "POST", CASES)?.body).toEqual({ runId: "run-reh", expectations: [{ kind: "never_calls", value: "crm.lookup" }] }),
    );
  });

  it("says, before keeping, that erasing the conversation will not reach the case", async () => {
    world();
    const dialog = await keepAsCase();
    expect(within(dialog).getByText(/Apagar a conversa não alcança um caso guardado/)).toBeInTheDocument();
  });

  it("offers no case while the rehearsal has not ended", async () => {
    world({ [`GET ${RUN}`]: () => ({ body: run({ phase: "awaiting_world", seq: 4 }) }), [`GET ${RUN}/steps`]: () => ({ body: { items: trail.slice(0, 4) } }),
      [`GET ${RUN}/steps/3/content`]: () => ({ body: { seq: 3, digest: "d", content: '{"id":"c-1"}' } }),
    });
    await openTab("Ensaio", atRehearsal);
    await screen.findByRole("region", { name: "A execução espera a resposta do mundo" });
    expect(screen.queryByRole("button", { name: "Guardar como caso" })).not.toBeInTheDocument();
  });

  it("says why a rehearsal could not be kept", async () => {
    world({ [`POST ${CASES}`]: () => ({ status: 409, body: { type: "fuseone:conflict", title: "Conflict", status: 409, detail: "call 1 (crm.lookup) was never answered" } }) });
    const dialog = await keepAsCase();
    await userEvent.click(within(dialog).getByRole("checkbox", { name: /nunca deveria chamar/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Guardar caso" }));
    expect(await screen.findByText(/never answered/)).toBeInTheDocument();
  });

  it("lists the corpus, naming what each case checks and where it came from", async () => {
    world({ [`GET ${CASES}`]: () => ({ body: { items: [kept] } }) });
    await openTab("Casos");
    const row = await screen.findByRole("listitem", { name: "case-1" });
    expect(within(row).getByText("crm.lookup")).toBeInTheDocument();
    expect(within(row).getByText("não consulta")).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: "run-reh" })).toHaveAttribute("href", "/runs/run-reh");
  });

  it("removes a case after saying it writes a new version and erases nothing", async () => {
    const calls = world({ [`GET ${CASES}`]: () => ({ body: { items: [kept] } }), [`DELETE ${CASES}/case-1`]: () => ({ status: 204 }) });
    await openTab("Casos");
    await userEvent.click(await screen.findByRole("button", { name: "Remover case-1" }));
    const confirm = await screen.findByRole("alertdialog");
    expect(within(confirm).getByText(/nova versão do conjunto de casos/)).toBeInTheDocument();
    await userEvent.click(within(confirm).getByRole("button", { name: "Remover" }));
    await waitFor(() => expect(sent(calls, "DELETE", `${CASES}/case-1`)).toBeDefined());
  });

  it("reads the corpus again once a case is removed", async () => {
    let gone = false;
    world({
      [`GET ${CASES}`]: () => ({ body: { items: gone ? [] : [kept] } }),
      [`DELETE ${CASES}/case-1`]: () => ((gone = true), { status: 204 }),
    });
    await openTab("Casos");
    await userEvent.click(await screen.findByRole("button", { name: "Remover case-1" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Remover" }));
    expect(await screen.findByText("Nenhum caso guardado")).toBeInTheDocument();
  });

  it("explains how a case is made while the agent has none", async () => {
    world({ [`GET ${CASES}`]: () => ({ status: 404 }) });
    await openTab("Casos");
    expect(await screen.findByText("Nenhum caso guardado")).toBeInTheDocument();
  });

  it("says why the corpus could not be read, and reads it again when asked", async () => {
    let fail = true;
    world({ [`GET ${CASES}`]: () => (fail ? ((fail = false), { status: 500 }) : { body: { items: [kept] } }) });
    await openTab("Casos");
    await userEvent.click(await screen.findByRole("button", { name: /tentar de novo/i }));
    expect(await screen.findByRole("listitem", { name: "case-1" })).toBeInTheDocument();
  });
});
