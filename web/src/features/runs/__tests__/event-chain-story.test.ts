import { describe, expect, it } from "vitest";
import { detailOf } from "@/features/runs/step-story";

/*
A chain of runs that events opened one after another stops at a limit
(#203). The trail says where a run stands in its chain, and a run at the end
says its events were not published — otherwise the next agent simply never
starts and nothing on the screen says why.
*/

describe("a run an event opened", () => {
  it("names its hop and the run that started the chain", () => {
    const line = detailOf({
      seq: 1,
      kind: "run_started",
      at: "2026-10-31T12:00:00Z",
      payload: { trigger: "event", chain: { root: "run-root", hop: 3 } },
    } as never);

    expect(line).toEqual({
      key: "runs.storyEventChain",
      values: { hop: 3, root: "run-root" },
    });
  });

  it("keeps the plain trigger line for a run no event opened", () => {
    const line = detailOf({
      seq: 1,
      kind: "run_started",
      at: "2026-10-31T12:00:00Z",
      payload: { trigger: "cron" },
    } as never);

    expect(line).toEqual({ key: "runs.storyTrigger", values: { trigger: "cron" } });
  });
});

describe("a run at the end of its chain", () => {
  it("says its events were withheld, with the root and the limit", () => {
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-10-31T12:00:00Z",
      payload: {
        outcome_ref: "content://x",
        reason: "finish_tool",
        events_withheld: { reason: "event_hop_limit", root: "run-root", hop: 8, limit: 8 },
      },
    } as never);

    expect(line).toEqual({
      key: "runs.eventsWithheld",
      values: { root: "run-root", limit: 8 },
    });
  });

  it("ignores a withheld record it cannot read", () => {
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-10-31T12:00:00Z",
      payload: { reason: "finish_tool", events_withheld: { reason: "event_hop_limit" } },
    } as never);

    expect(line.key).toBe("runs.finishedByFinish");
  });
});
