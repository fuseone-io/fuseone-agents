import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve, type Routes } from "@/features/workbench/__tests__/workbench-fixtures";
import type { Run, Step } from "@/lib/api/client";

/*
A trace in the rehearsal tab (NT-018 phase 5c): a run opened to be read, from a regression
of an experiment. It is strictly read-only, wins over a rehearsal named beside it, and a
run that cannot be read says so without falling back to a rehearsal.
*/

const run = (runId: string): Run => ({
  runId, scope: { company: "acme", area: "cx" }, agentId: "triagem", versionId: "v2abc", phase: "finished", seq: 2, startedAt: "2026-11-20T12:00:00Z", cost: { micros: 0 },
});
const steps = (): Step[] => [
  { seq: 1, kind: "run_started", at: "2026-11-20T12:00:00Z", hash: "h1", payload: {} },
  { seq: 2, kind: "run_finished", at: "2026-11-20T12:00:00Z", hash: "h2", payload: {} },
];
const seal = { version: "v2abc", sealId: "s-1", company: "acme", area: "cx", sealedBy: "usr_ana", sealedAt: "2026-11-19T12:00:00Z" };

function world(over: Routes = {}) {
  return serve({
    ["GET /agents/triagem/draft"]: () => ({ body: draftOf(), etag: '"e1"' }),
    ["GET /agents/triagem/draft/turns"]: () => ({ body: { items: [], conversation: "open" } }),
    ["GET /agents/triagem/candidates"]: () => ({ body: { items: [seal] } }),
    ["GET /runs/run-case-a"]: () => ({ body: run("run-case-a") }),
    ["GET /runs/run-case-a/steps"]: () => ({ body: { items: steps() } }),
    ["GET /runs/run-reh"]: () => ({ body: run("run-reh") }),
    ["GET /runs/run-reh/steps"]: () => ({ body: { items: steps() } }),
    ...over,
  });
}

describe("a trace in the rehearsal tab", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("wins over a rehearsal named beside it, and offers nothing to do", async () => {
    const calls = world();
    openAt("/agents/triagem/workbench?rehearsal=run-reh&trace=run-case-a");
    expect(await screen.findByText("run-case-a")).toBeInTheDocument();
    // The rehearsal, finished, would offer to be kept: shown, its button would be here.
    expect(screen.queryByRole("button", { name: "Guardar como caso" })).not.toBeInTheDocument();
    expect(calls.some((c) => c.path === "/runs/run-reh")).toBe(false);
  });

  it("says the run could not be read, and stays read-only", async () => {
    world({ ["GET /runs/run-gone"]: () => ({ status: 404, body: { title: "not found", status: 404 } }) });
    openAt("/agents/triagem/workbench?rehearsal=run-reh&trace=run-gone");
    expect(await screen.findByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Guardar como caso" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Abrir ensaio" })).not.toBeInTheDocument();
  });

  it("closes back to the rehearsal tab as it was", async () => {
    world();
    openAt("/agents/triagem/workbench?trace=run-case-a");
    await userEvent.click(await screen.findByRole("button", { name: "Fechar trace" }));
    expect(await screen.findByRole("button", { name: "Abrir ensaio" })).toBeInTheDocument();
  });
});
