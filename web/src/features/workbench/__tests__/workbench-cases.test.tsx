import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve, type Call, type Routes } from "@/features/workbench/__tests__/workbench-fixtures";
import type { Run, Step } from "@/lib/api/client";

/*
Cases in the workbench (NT-018 phase 5b). An ended rehearsal is kept as a case with the
world its author answered for, and keeping it says that erasing the conversation will not
reach the case (D2).
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

async function openTab(name: string, at: string) {
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

});
