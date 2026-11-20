import { describe, expect, it } from "vitest";
import { buildGraph } from "@/features/runs/run-graph";
import { detailOf } from "@/features/runs/step-story";
import { buildTrail } from "@/features/runs/trail-model";
import type { Step, StepKind } from "@/lib/api/client";

/*
A live rehearsal's steps, as the run's trail and diagram show them (NT-018 phase 4). The
console reads them one release before anything writes them: the author's decision closes
the Gate's request as a person's does, the wait and the answer are the author's part of the
story, and each says what happened in words.
*/

let seq = 0;
const step = (kind: StepKind, payload: Record<string, unknown> = {}, at = "2026-11-20T10:00:00Z"): Step =>
  ({ seq: ++seq, kind, at, hash: "", payload }) as Step;

describe("a rehearsal's steps", () => {
  it("pairs the Gate's request with the author's decision, carrying the wait", () => {
    const { nodes } = buildGraph([
      step("approval_requested", {}, "2026-11-20T10:00:00Z"),
      step("rehearsal_decided", { approved: true, by: "usr_ana", at_seq: 1 }, "2026-11-20T10:02:00Z"),
    ]);
    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({ kind: "human", latencyMs: 120_000 });
  });

  it("places the wait, the answer and the decision in the author's part of the trail", () => {
    const groups = buildTrail(
      [
        step("run_started"),
        step("tool_called", { tool: "crm.lookup", effect: 1 }),
        step("world_awaited", { tool: "crm.lookup", call_seq: 2 }),
        step("world_supplied", { at_seq: 3, ref: "r", digest: "d", by: "usr_ana" }),
      ],
      { filter: "all" },
    );
    const human = groups.find((g) => g.phase === "human");
    const kinds = human?.entries.map((e) => (e.kind === "step" ? e.step.kind : "fold"));
    expect(kinds).toEqual(["world_awaited", "world_supplied"]);
  });

  it("tells who answered for the world and who decided, and what was decided", () => {
    expect(detailOf(step("world_awaited", { tool: "crm.lookup", call_seq: 2 }))).toEqual({
      key: "runs.storyWorldAwaited", values: { tool: "crm.lookup" },
    });
    expect(detailOf(step("world_supplied", { at_seq: 3, by: "usr_ana" }))).toEqual({
      key: "runs.storyWorldSupplied", values: { who: "usr_ana" },
    });
    expect(detailOf(step("rehearsal_decided", { approved: false, by: "usr_ana" }))).toEqual({
      key: "runs.storyRehearsalRefusedBy", values: { who: "usr_ana" },
    });
  });
});
