import { describe, expect, it } from "vitest";
import { detailOf, line, titleOf } from "@/features/runs/step-story";
import { verbOf } from "@/features/runs/step-verb";
import { tileOf } from "@/features/runs/trail-icon";
import { i18next } from "@/i18n";
import type { Step } from "@/lib/api/client";

const linked = {
  seq: 14,
  kind: "work_item_linked",
  at: "2026-11-02T12:00:00Z",
  hash: "h",
  payload: {
    alias: "github:fuseone-io/fuseone-agents#240",
    root: "github:fuseone-io/fuseone-agents#204",
    proof: { kind: "tool_returned", seq: 12 },
  },
} as Step;

/*
 * A Developer run opens a pull request for the card it claimed and records
 * the PR as another name of that card (#204). The run trail says which PR
 * was linked to which card, in words, in both languages.
 */
describe("a run that linked a pull request to its work item", () => {
  it("names the alias and the root it was linked to", () => {
    expect(detailOf(linked)).toEqual({
      key: "runs.storyWorkItemLinkedDetail",
      values: {
        alias: "github:fuseone-io/fuseone-agents#240",
        root: "github:fuseone-io/fuseone-agents#204",
      },
    });
  });

  it("titles, verbs and draws the step", () => {
    expect(titleOf(linked)).toBe("runs.storyWorkItemLinked");
    expect(verbOf(linked).verb).toBe("runs.verbLinked");
    expect(tileOf(linked, false).icon).toBeDefined();
  });

  it.each([
    [
      "en-US",
      "Linked github:fuseone-io/fuseone-agents#240 to github:fuseone-io/fuseone-agents#204",
      "Linked a pull request to the work item",
      "linked a pull request",
    ],
    [
      "pt-BR",
      "Vinculou github:fuseone-io/fuseone-agents#240 a github:fuseone-io/fuseone-agents#204",
      "Vinculou um pull request ao item de trabalho",
      "vinculou um pull request",
    ],
  ])("reads in %s", (locale, detail, title, verb) => {
    const t = i18next.getFixedT(locale);
    expect(line(detailOf(linked), t)).toBe(detail);
    expect(t(titleOf(linked))).toBe(title);
    expect(t(verbOf(linked).verb)).toBe(verb);
  });

  it("says nothing it cannot read from a payload it does not recognise", () => {
    const odd = { ...linked, payload: { alias: 240 } } as unknown as Step;
    expect(detailOf(odd)).toEqual({
      key: "runs.storyWorkItemLinkedDetail",
      values: { alias: "", root: "" },
    });
  });
});
