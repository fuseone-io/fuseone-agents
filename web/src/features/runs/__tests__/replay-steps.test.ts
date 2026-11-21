import { describe, expect, it } from "vitest";
import { detailOf, titleOf } from "@/features/runs/step-story";
import { verbOf } from "@/features/runs/step-verb";
import type { Step } from "@/lib/api/client";

/*
A replayed case's divergence, as a run's trail tells it (NT-018 phase 5). The console reads
the step one release before anything writes it: the first call that left the recorded world,
said in words, with the tool and where the world stood.
*/

const diverged = (divergence: string, tool = "kb.search"): Step =>
  ({ seq: 9, kind: "replay_diverged", at: "2026-11-20T10:00:00Z", hash: "", payload: { position: 2, divergence, tool } }) as Step;

describe("a replayed case's divergence", () => {
  it("says how the run left its world, naming the tool and the position", () => {
    for (const divergence of ["missing", "extra", "repeated", "out_of_order"]) {
      expect(detailOf(diverged(divergence))).toEqual({
        key: `runs.storyReplayDiverged_${divergence}`, values: { tool: "kb.search", position: 2 },
      });
    }
  });

  it("titles and names the step as a warning", () => {
    expect(titleOf(diverged("extra"))).toBe("runs.storyReplayDivergedTitle");
    expect(verbOf(diverged("extra"))).toEqual({ verb: "runs.verbReplayDiverged", tone: "warn" });
  });
});
