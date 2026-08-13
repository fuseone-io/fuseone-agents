import { describe, expect, it } from "vitest";
import { draftSentence } from "@/features/policies/policy-sentence";
import { changesBetween } from "@/features/policies/policy-form";
import type { PolicyInput } from "@/lib/api/client";

function draft(over: Partial<PolicyInput> = {}): PolicyInput {
  return { name: "x", effect: "deny", mode: "enforce", ...over };
}

/**
 * The words come from the catalogue, so the tests assert the shape: which
 * parts, in which order, joined how. A test pinning the Portuguese would fail
 * the day somebody improves the wording and pass the day the sentence stops
 * making sense.
 */
const t = (key: string) => key;

describe("the sentence", () => {
  it("reads the rule the author is building", () => {
    const got = draftSentence(
      draft({
        resource: "customer.*",
        effects: ["read", "write"],
        conditions: [{ field: "args.rows", op: "gt", value: "100" }],
      }),
      t,
    );

    expect(got).toBe(
      "customer.* · effect.read, effect.write · args.rows > 100 → verdict.block",
    );
  });

  it("reads the same for a draft and for a stored rule", () => {
    // One renderer. The server used to compose this too, which meant two
    // renderings of one structure — and the stored one arrived in whichever
    // language the binary held.
    const stored = draft({ resource: "crm.reply", effects: ["write"] });

    expect(draftSentence(stored, t)).toBe(
      "crm.reply · effect.write → verdict.block",
    );
  });

  it("says when the rule will not be enforcing", () => {
    // A rule that reads "→ deny" while denying nothing is the most misleading
    // thing this screen could show.
    expect(draftSentence(draft({ mode: "monitor" }), t)).toContain(
      "policies.onlyMonitoring",
    );
  });

  it("covers everything when no resource is chosen yet", () => {
    // A half-written rule must not read as narrower than it is.
    expect(draftSentence(draft(), t)).toBe("* → verdict.block");
  });
});

describe("what is about to change", () => {
  it("names each field, with what it was and what it becomes", () => {
    // Saving a rule is a governance act. Somebody should see that widening
    // the reach is part of what they are about to do, rather than discovering
    // it from the runs it stops.
    const before = draft({ mode: "monitor", resource: "crm.reply" });
    const after = draft({ mode: "enforce", resource: "crm.*" });

    const changes = changesBetween(before, after);
    expect(changes).toEqual([
      { field: "policies.fieldResource", from: "crm.reply", to: "crm.*" },
      { field: "policies.fieldEnforcement", from: "monitor", to: "enforce" },
    ]);
  });

  it("reports nothing when nothing moved", () => {
    expect(changesBetween(draft(), draft())).toEqual([]);
  });

  it("sees a condition that changed inside a list", () => {
    // A rule whose threshold moved from 100 to 10 is a different rule, and a
    // diff that only counted list length would call it unchanged.
    const before = draft({
      conditions: [{ field: "args.rows", op: "gt", value: "100" }],
    });
    const after = draft({
      conditions: [{ field: "args.rows", op: "gt", value: "10" }],
    });

    expect(changesBetween(before, after)).toHaveLength(1);
  });
});
