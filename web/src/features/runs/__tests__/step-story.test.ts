import { describe, expect, it } from "vitest";
import { detailOf, summaryOf } from "@/features/runs/step-story";
import type { Step } from "@/lib/api/client";

/*
What a finished run says in the trail.

The exception is the author's words and the trail says the agent asserted
them, never that anything checked: the condition is a sentence about the
world, and the platform has no way to evaluate one.
*/

describe("a run that stopped where the author said it would", () => {
  it("quotes the exception, and keeps what the agent said about it", () => {
    // The author's words, and the trail says the agent asserted them rather
    // than that anything checked: the condition is a sentence about the
    // world, and the platform has no way to evaluate one.
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-08-14T12:00:00Z",
      payload: {
        outcome: "Procurei pelos dois e-mails.",
        stopped_by: "não encontrar o cliente",
      },
    } as never);

    expect(line.key).toBe("runs.stoppedByException");
    expect(line.values).toMatchObject({ what: "não encontrar o cliente" });
  });

  it("does not infer why an old run finished", () => {
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-08-14T12:00:00Z",
      payload: { outcome: "Respondi e encerrei." },
    } as never);

    expect(line.key).toBe("runs.finishedWithOutcome");
    expect(line.values).toMatchObject({ outcome: "Respondi e encerrei." });
  });

  it("says no tool call finished the run only when the ledger recorded it", () => {
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-08-14T12:00:00Z",
      payload: { outcome: "Respondi e encerrei.", reason: "no_tool_call" },
    } as never);

    expect(line.key).toBe("runs.finishedByNoToolCallWithOutcome");
    expect(line.values).toMatchObject({ outcome: "Respondi e encerrei." });
  });

  it("says the explicit finish action ended the run when recorded", () => {
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-08-14T12:00:00Z",
      payload: { outcome: "Diagnóstico concluído.", reason: "finish_tool" },
    } as never);

    expect(line.key).toBe("runs.finishedByFinishWithOutcome");
    expect(line.values).toMatchObject({ outcome: "Diagnóstico concluído." });
  });
});

describe("a run finished since the answer moved", () => {
  it("says the answer is held rather than reading an old run as silence", () => {
    // Blank would be the story of an agent that finished saying nothing, which
    // is a different run from one whose answer is in the content store.
    expect(
      detailOf({
        seq: 4,
        kind: "run_finished",
        at: "2026-08-18T12:00:00Z",
        hash: "h",
        payload: { outcome_ref: "content:run-1:4" },
      } as Step),
    ).toEqual({ key: "runs.outcomeStored" });
  });

  it("says no tool call finished the run and the answer is held when recorded", () => {
    expect(
      detailOf({
        seq: 4,
        kind: "run_finished",
        at: "2026-08-18T12:00:00Z",
        hash: "h",
        payload: { outcome_ref: "content:run-1:4", reason: "no_tool_call" },
      } as Step),
    ).toEqual({ key: "runs.finishedByNoToolCallStored" });
  });

  it("says the answer is held after an explicit finish action", () => {
    expect(
      detailOf({
        seq: 4,
        kind: "run_finished",
        at: "2026-08-18T12:00:00Z",
        hash: "h",
        payload: { outcome_ref: "content:run-1:4", reason: "finish_tool" },
      } as Step),
    ).toEqual({ key: "runs.finishedByFinishStored" });
  });
});

describe("a run parked because the model only returned text", () => {
  it("says a person must inspect the missing finish action", () => {
    expect(
      detailOf({
        seq: 7,
        kind: "parked",
        at: "2026-08-20T12:00:00Z",
        hash: "h",
        payload: { reason: "no_finish_action" },
      } as Step),
    ).toEqual({ key: "runs.storyNoFinishAction" });
  });
});

describe("a run parked by a ceiling", () => {
  it("names the budget dimension instead of making a zero-cost run look impossible", () => {
    const line = detailOf({
      seq: 63,
      kind: "gate_decided",
      at: "2026-08-20T12:00:00Z",
      hash: "h",
      payload: {
        tool: "grafana.query_prometheus",
        verdict: "block",
        rule: "budget",
        breached: "steps",
        budget: { micros: 1_000_000, steps: 60 },
        committed: { steps: 60 },
        estimate: { tool_calls: 1, steps: 1 },
        projected: { steps: 61 },
      },
    } as never);

    expect(line.key).toBe("runs.storyBudgetExceededSteps");
    expect(line.values).toMatchObject({
      used: "61",
      ceiling: "60",
      already: "60",
      requested: "1",
    });
  });
});

describe("a cached tool result", () => {
  it("says the server was not called for this answer", () => {
    const step = {
      seq: 8,
      kind: "tool_returned",
      at: "2026-08-22T12:00:00Z",
      hash: "h",
      payload: {
        tool: "grafana.query_prometheus",
        cached: true,
        cached_from_run: "run-original",
        cached_from_seq: 4,
      },
    } as never;

    expect(detailOf(step)).toEqual({
      key: "runs.storyToolCached",
      values: { run: "run-original", seq: 4 },
    });
    expect(summaryOf(step)).toEqual({
      key: "runs.summaryToolCached",
      values: { tool: "grafana.query_prometheus" },
    });
  });
});

describe("a planned turn", () => {
  it("shows prompt composition as measured content, not cost", () => {
    const line = detailOf({
      seq: 2,
      kind: "planned",
      at: "2026-08-22T12:00:00Z",
      hash: "h",
      payload: {
        prompt: {
          unit: "content_bytes",
          total: 4096,
          instructions: 512,
          input: 128,
          tool_results: 2048,
        },
      },
    } as never);

    expect(line).toEqual({
      key: "runs.storyPromptComposition",
      values: {
        total: "4 KiB",
        instructions: "512 B",
        input: "128 B",
        toolResults: "2 KiB",
      },
    });
  });
});
