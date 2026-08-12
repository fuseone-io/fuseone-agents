import { describe, expect, it } from "vitest";
import { tallyOf } from "@/features/policies/policy-tally";
import type { Policy } from "@/lib/api/client";

function policy(over: Partial<Policy>): Policy {
  return {
    code: "POL-1", name: "x", sentence: "x → negar",
    effect: "deny", mode: "enforce", enabled: true,
    ...over,
  };
}

describe("the policy tally", () => {
  it("counts enforcing, monitoring and disabled as three different things", () => {
    // A monitored rule is not off and not in force. Folding either pair
    // together would make the screen report a governance state that is not
    // the one the installation is in.
    const tally = tallyOf([
      policy({ code: "A" }),
      policy({ code: "B", mode: "monitor" }),
      policy({ code: "C", enabled: false }),
    ]);

    expect(tally).toMatchObject({ enforcing: 1, monitoring: 1, disabled: 1 });
  });

  it("attributes decisions to the kind the rule produces", () => {
    // A policy has one effect, so its hits are hits of that kind. Nothing is
    // inferred and no second count is invented.
    const tally = tallyOf([
      policy({ code: "A", effect: "deny", hits: 7 }),
      policy({ code: "B", effect: "escalate", hits: 3 }),
      policy({ code: "C", effect: "allow", hits: 2 }),
    ]);

    expect(tally).toMatchObject({ hits: 12, denied: 7, escalated: 3 });
  });

  it("counts a rule that never fired as zero rather than skipping it", () => {
    const tally = tallyOf([policy({ code: "A" })]);
    expect(tally.hits).toBe(0);
    expect(tally.enforcing).toBe(1);
  });

  it("counts a disabled rule's past decisions, because they happened", () => {
    // Turning a rule off does not unmake what it did while it was on, and a
    // screen that hid those would misreport the period.
    const tally = tallyOf([policy({ code: "A", enabled: false, hits: 5 })]);
    expect(tally.hits).toBe(5);
  });
});
