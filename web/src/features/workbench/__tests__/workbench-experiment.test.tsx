import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, PUBLISHED, serve, type Call, type Routes } from "@/features/workbench/__tests__/workbench-fixtures";
import type { ExperimentCaseChange, ExperimentListing, ExperimentReport } from "@/features/experiments/experiments-api";
import type { Run, Step } from "@/lib/api/client";

/*
The workbench's experiment tab (NT-018 phase 5c). A candidate runs against the agent's
cases and is compared with the published version on the same dataset version. The
published side is an ended experiment with exactly the same pins when one exists, and a
new one when none does; the pins are read once, when the person asks. Each regression
opens its run in the rehearsal tab, read-only.
*/

const DRAFT = "/agents/triagem/draft";
const EXPERIMENTS = "/agents/triagem/experiments";
const COMPARISON = "/agents/triagem/experiment-comparison";
const seal = { version: "v2abc", sealId: "s-1", company: "acme", area: "cx", sealedBy: "usr_ana", sealedAt: "2026-11-19T12:00:00Z" };

const listing = (id: string, over: Partial<ExperimentListing> = {}): ExperimentListing => ({
  id, openedAt: "2026-11-20T12:00:00Z", datasetVersion: 3, version: "v1", config: {}, repetitions: 1, runs: 4, cases: 4, ...over,
});
const report = (id: string, over: { running?: boolean; version?: string } = {}): ExperimentReport => ({
  summary: {
    id, datasetVersion: 3, version: over.version ?? "v1", config: {}, casesRun: 4, casesInVersion: 4, erased: 0, repetitions: 1,
    held: 3, broken: 1, notComparable: 0, cost: { micros: 0 }, running: over.running ?? false,
  },
  cases: ["case-a", "case-b", "case-c", "case-d"].map((c) => ({
    case: { id: c, runId: `run-${c}`, settled: "finished", steps: 3, cost: { micros: 0 } }, brokenIn: 0, divergedIn: 0, ran: 1,
  })),
});
const change = (id: string, was: ExperimentCaseChange["was"], now: ExperimentCaseChange["now"]): ExperimentCaseChange => ({
  id, was, now, costMicros: 0, steps: 3, wasBrokenIn: was === "broke" ? 1 : 0, wasRan: 1, nowBrokenIn: now === "broke" ? 1 : 0, nowRan: 1, wasDivergedIn: 0, nowDivergedIn: 0,
});
const comparison = (from: string, to: string) => ({
  from: report(from).summary, to: report(to, { version: "v2abc" }).summary, sharedOnly: false, onlyFrom: 0, onlyTo: 0, regressed: 1, fixed: 1, costMicros: 0,
  cases: [change("case-a", "held", "broke"), change("case-b", "broke", "held"), change("case-c", "held", "held"), change("case-d", "not_comparable", "held")],
});
const run = (runId: string): Run => ({
  runId, scope: { company: "acme", area: "cx" }, agentId: "triagem", versionId: "v2abc", phase: "finished", seq: 2, startedAt: "2026-11-20T12:00:00Z", cost: { micros: 0 },
});
const steps = (): Step[] => [
  { seq: 1, kind: "run_started", at: "2026-11-20T12:00:00Z", hash: "h1", payload: {} },
  { seq: 2, kind: "run_finished", at: "2026-11-20T12:00:00Z", hash: "h2", payload: {} },
];

const accepted = (id: string, datasetVersion = 3) => ({ status: 202, body: { id, cases: 4, repetitions: 1, runs: 4, datasetVersion } });

/** The candidate opens as exp-cand; a published-version experiment the screen opens is exp-ref-new. */
function world(over: Routes = {}) {
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
    ["GET /agents/triagem"]: () => ({ body: PUBLISHED }),
    ["GET /agents/triagem/candidates"]: () => ({ body: { items: [seal] } }),
    [`POST ${EXPERIMENTS}`]: (call) => accepted((call.body as { sealId?: string }).sealId ? "exp-cand" : "exp-ref-new"),
    [`GET ${EXPERIMENTS}`]: () => ({ body: { experiments: [] } }),
    [`GET ${EXPERIMENTS}/exp-cand`]: () => ({ body: report("exp-cand", { version: "v2abc" }) }),
    [`GET ${EXPERIMENTS}/exp-ref-new`]: () => ({ body: report("exp-ref-new") }),
    [`GET ${EXPERIMENTS}/exp-ref-old`]: () => ({ body: report("exp-ref-old") }),
    [`GET ${EXPERIMENTS}/exp-ref-live`]: () => ({ body: report("exp-ref-live", { running: true }) }),
    // Ended, so only their pins can keep them from being the baseline.
    ...Object.fromEntries(["dataset", "config", "reps", "version"].map((k) => [`GET ${EXPERIMENTS}/exp-other-${k}`, () => ({ body: report(`exp-other-${k}`) })])),
    [`GET ${COMPARISON}`]: (call) => ({ body: comparison(call.query.get("from") ?? "", call.query.get("to") ?? "") }),
    ["GET /runs/run-case-a"]: () => ({ body: run("run-case-a") }),
    ["GET /runs/run-case-a/steps"]: () => ({ body: { items: steps() } }),
    ["GET /runs/run-reh"]: () => ({ body: run("run-reh") }),
    ["GET /runs/run-reh/steps"]: () => ({ body: { items: steps() } }),
    ...over,
  });
}

const posts = (calls: Call[]) => calls.filter((c) => c.method === "POST" && c.path === EXPERIMENTS).map((c) => c.body);
const compared = (calls: Call[]) => calls.filter((c) => c.method === "GET" && c.path === COMPARISON).map((c) => [c.query.get("from"), c.query.get("to")]);
const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function runAgainstCases() {
  openAt("/agents/triagem/workbench");
  await userEvent.click(await screen.findByRole("tab", { name: "Experimento" }));
  await userEvent.click(await screen.findByRole("button", { name: "Rodar contra os casos" }));
}

describe("the workbench's experiment", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("runs the candidate on the newest dataset version with its own configuration", async () => {
    const calls = world();
    await runAgainstCases();
    await waitFor(() => expect(posts(calls)[0]).toEqual({ sealId: "s-1", repetitions: 1 }));
  });

  it("reuses the newest ended experiment of the published version with exactly the same pins", async () => {
    const calls = world({
      [`GET ${EXPERIMENTS}`]: () => ({ body: { experiments: [
        // The candidate's own, as when its text is the published one: never its own baseline.
        listing("exp-cand"),
        listing("exp-ref-live"),
        listing("exp-other-dataset", { datasetVersion: 4 }),
        listing("exp-other-config", { config: { model: "m2" } }),
        listing("exp-other-reps", { repetitions: 2 }),
        listing("exp-other-version", { version: "v0" }),
        listing("exp-ref-old"),
      ] } }),
    });
    await runAgainstCases();
    expect(await screen.findByText("Referência reaproveitada: exp-ref-old")).toBeInTheDocument();
    expect(posts(calls)).toHaveLength(1);
    await waitFor(() => expect(compared(calls)).toContainEqual(["exp-ref-old", "exp-cand"]));
  });

  it("opens an experiment of the published version, pinned to the candidate's dataset version, when none matches", async () => {
    const calls = world({
      [`GET ${EXPERIMENTS}`]: () => ({ body: { experiments: [listing("exp-ref-live"), listing("exp-other-dataset", { datasetVersion: 4 })] } }),
    });
    await runAgainstCases();
    expect(await screen.findByText("Referência aberta: exp-ref-new")).toBeInTheDocument();
    expect(posts(calls)[1]).toEqual({ version: "v1", datasetVersion: 3, repetitions: 1 });
    await waitFor(() => expect(compared(calls)).toContainEqual(["exp-ref-new", "exp-cand"]));
  });

  it("keeps the published version it read when the person asked, though another is published meanwhile", async () => {
    let published = PUBLISHED;
    const calls = world({
      ["GET /agents/triagem"]: () => ({ body: published }),
      [`POST ${EXPERIMENTS}`]: (call) => {
        if (!(call.body as { sealId?: string }).sealId) return accepted("exp-ref-new");
        published = { ...PUBLISHED, agent: { ...PUBLISHED.agent, versionId: "v9" } };
        return accepted("exp-cand");
      },
      [`GET ${EXPERIMENTS}`]: () => ({ body: { experiments: [listing("exp-v9", { version: "v9" })] } }),
    });
    await runAgainstCases();
    await waitFor(() => expect(posts(calls)[1]).toEqual({ version: "v1", datasetVersion: 3, repetitions: 1 }));
  });

  it("asks again under the same keys after the baseline failed to open, reaching the candidate already opened", async () => {
    let failing = true;
    const calls = world({
      [`POST ${EXPERIMENTS}`]: (call) => {
        if ((call.body as { sealId?: string }).sealId) return accepted("exp-cand");
        return failing ? { status: 503 } : accepted("exp-ref-new");
      },
    });
    await runAgainstCases();
    await waitFor(() => expect(posts(calls)).toHaveLength(2));
    failing = false;
    await userEvent.click(await screen.findByRole("button", { name: "Rodar contra os casos" }));
    await waitFor(() => expect(posts(calls)).toHaveLength(4));
    const keys = calls.filter((c) => c.method === "POST" && c.path === EXPERIMENTS).map((c) => c.headers.get("Idempotency-Key"));
    expect([keys[2], keys[3]]).toEqual([keys[0], keys[1]]);
  });

  it("shows the candidate's report alone for an agent never published", async () => {
    const calls = world({ ["GET /agents/triagem"]: () => ({ status: 404 }) });
    await runAgainstCases();
    expect(await screen.findByText(/Sem versão publicada para comparar/)).toBeInTheDocument();
    expect(posts(calls)).toHaveLength(1);
    expect(calls.some((c) => c.path === COMPARISON)).toBe(false);
  });

  it("compares only once both experiments have ended", async () => {
    let running = true;
    const calls = world({
      [`GET ${EXPERIMENTS}/exp-cand`]: () => ({ body: report("exp-cand", { version: "v2abc", running }) }),
    });
    await runAgainstCases();
    expect(await screen.findByText(/Ainda rodando/)).toBeInTheDocument();
    await pause(50);
    expect(compared(calls)).toEqual([]);
    running = false;
    await waitFor(() => expect(compared(calls)).toEqual([["exp-ref-new", "exp-cand"]]), { timeout: 4000 });
  });

  it("groups the cases into regression, fixed, kept, and apart, with no judge column", async () => {
    world();
    await runAgainstCases();
    const regressions = await screen.findByRole("region", { name: /Regressões/ });
    expect(within(regressions).getByText("case-a")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: /Corrigidos/ })).getByText("case-b")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: /Mantidos/ })).getByText("case-c")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: /À parte/ })).getByText("case-d")).toBeInTheDocument();
    expect(screen.queryByText(/Juiz/)).not.toBeInTheDocument();
  });

  // D3: a corpus recorded in another scope is answered as absent, by the recorded scope.
  it("opens nothing more when the candidate is refused for a corpus recorded in another scope", async () => {
    const calls = world({ [`POST ${EXPERIMENTS}`]: () => ({ status: 404, body: { type: "fuseone:not-found", title: "Not found", status: 404, detail: "exp-x" } }) });
    await runAgainstCases();
    expect(await screen.findByText("Não encontrado: exp-x")).toBeInTheDocument();
    expect(posts(calls)).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Rodar contra os casos" })).toBeInTheDocument();
  });

  it("opens a regression's run in the rehearsal tab, read-only", async () => {
    world();
    await runAgainstCases();
    const regressions = await screen.findByRole("region", { name: /Regressões/ });
    await userEvent.click(within(regressions).getByRole("button", { name: /Ver trace de case-a/ }));
    expect(await screen.findByRole("tab", { name: "Ensaio", selected: true })).toBeInTheDocument();
    expect(await screen.findByText("run-case-a")).toBeInTheDocument();
    expect(screen.getByText(/somente leitura/)).toBeInTheDocument();
    for (const name of ["Guardar como caso", "Abandonar ensaio", "Novo ensaio"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
  });
});
