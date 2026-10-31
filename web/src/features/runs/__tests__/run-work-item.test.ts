import { describe, expect, it } from "vitest";
import { detailOf, titleOf } from "@/features/runs/step-story";
import { verbOf } from "@/features/runs/step-verb";
import type { Step } from "@/lib/api/client";

const claimed = {
  seq: 5,
  kind: "work_item_claimed",
  at: "2026-10-30T12:00:00Z",
  hash: "h",
  payload: { ref: "github:fuseone-io/fuseone-agents#123" },
} as Step;

/*
 * A factory run claims its card first, once it has chosen one. The trail
 * has to name the card, or "which run worked on #123" cannot be read from the screen.
 */
describe("a run that claimed a work item", () => {
  it("names the card it claimed", () => {
    expect(detailOf(claimed)).toEqual({
      key: "runs.storyWorkItemClaimedDetail",
      values: { ref: "github:fuseone-io/fuseone-agents#123" },
    });
  });

  it("titles and verbs the step in words", () => {
    expect(titleOf(claimed)).toBe("runs.storyWorkItemClaimed");
    expect(verbOf(claimed).verb).toBe("runs.verbClaimed");
  });
});
