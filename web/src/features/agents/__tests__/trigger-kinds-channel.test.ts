import { describe, expect, it } from "vitest";
import { fieldOf, incomplete, TRIGGER_KINDS } from "@/features/agents/trigger-kinds";

/*
A conversation is a fourth way in, and the one that carries nothing.

The other three are half-filled rows until somebody types the expression, the
path or the event name — which is the worst shape a trigger can have, because
the screen says the agent is triggered and nothing starts it. A channel
trigger is complete the moment it exists: what it needs is a conversation
mapped to its scope, and that is administrative, not the author's to type.
*/
describe("starting from a conversation", () => {
  it("is one of the kinds an author may choose", () => {
    expect(TRIGGER_KINDS).toContain("channel");
  });

  it("is complete with nothing filled in", () => {
    expect(incomplete({ type: "channel" })).toBe(false);
  });

  // The other three are not, and that check has to keep working.
  it("still calls an unfilled schedule incomplete", () => {
    expect(incomplete({ type: "cron" })).toBe(true);
  });

  it("asks for no field", () => {
    expect(fieldOf("channel")).toBeUndefined();
  });
});
