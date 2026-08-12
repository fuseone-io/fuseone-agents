import { describe, expect, it } from "vitest";
import { draftSentence } from "@/features/policies/policy-sentence";
import { changesBetween } from "@/features/policies/policy-form";
import type { PolicyInput } from "@/lib/api/client";

function draft(over: Partial<PolicyInput> = {}): PolicyInput {
  return { name: "x", effect: "deny", mode: "enforce", ...over };
}

describe("the draft sentence", () => {
  it("reads the rule the author is building", () => {
    const got = draftSentence(
      draft({
        resource: "customer.*",
        effects: ["read", "write"],
        conditions: [{ field: "args.rows", op: "gt", value: "100" }],
      }),
    );

    expect(got).toBe("customer.* · read, write · args.rows > 100 → negar");
  });

  it("matches what the server renders for the same rule", () => {
    // The two are separate implementations of one thing, which is the risk
    // this test exists to bound. The server's version is the one shown beside
    // a stored rule; this one only exists while a draft has been nowhere.
    const got = draftSentence(
      draft({ resource: "crm.reply", effects: ["write"] }),
    );

    // Taken from the Go side's own test: internal/domain/policy_test.go.
    expect(got).toBe("crm.reply · write → negar");
  });

  it("says when the rule will not be enforcing", () => {
    expect(draftSentence(draft({ mode: "monitor" }))).toContain("monitorando");
  });

  it("covers everything when no resource is chosen yet", () => {
    // A half-written rule must not read as narrower than it is.
    expect(draftSentence(draft())).toBe("* → negar");
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
      { field: "recurso", from: "crm.reply", to: "crm.*" },
      { field: "aplicação", from: "monitor", to: "enforce" },
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
