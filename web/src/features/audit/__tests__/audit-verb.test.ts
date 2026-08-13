import { describe, expect, it } from "vitest";
import { verbOf } from "@/features/audit/audit-verb";

describe("the trail's verbs", () => {
  it("names the translation to read, not the word itself", () => {
    // The map held Portuguese, so an installation running in English read
    // "bloqueou" in its audit trail. The word is chosen where it is rendered.
    expect(verbOf("gate.blocked").label).toBe("audit.verbBlocked");
  });

  it("shows an unknown verb as itself rather than as a blank", () => {
    // The trail is append-only and outlives the console. An entry written by a
    // version that knew a verb this one does not must stay readable, and
    // showing nothing would be the console quietly editing history. The action
    // name is its own label, and a missing translation renders as itself.
    expect(verbOf("policy.rewritten").label).toBe("policy.rewritten");
  });

  it("colours only the decisions", () => {
    // A trail where every row is coloured says nothing about where to look.
    expect(verbOf("tool.classified").className).toContain("muted");
    expect(verbOf("gate.blocked").className).toContain("danger");
  });
});
