import { describe, expect, it } from "vitest";
import { i18next } from "@/i18n";
import { eventCycleMessage } from "@/features/agents/event-cycle-warning";

const pt = i18next.getFixedT("pt-BR");
const en = i18next.getFixedT("en-US");

describe("the warning a publish that closes an event cycle shows", () => {
  it("names every agent in the cycle and comes back to the first", () => {
    const message = eventCycleMessage(
      { cycles: [["dev", "revisor", "qa"]], hopLimit: 8 },
      pt,
      "pt-BR",
    );

    expect(message).toContain("dev → revisor → qa → dev");
    expect(message).toContain("8 saltos");
  });

  it("says the same in English", () => {
    const message = eventCycleMessage(
      { cycles: [["dev", "reviewer"]], hopLimit: 8 },
      en,
      "en-US",
    );

    expect(message).toContain("dev → reviewer → dev");
    expect(message).toContain("8 hops");
  });

  it("lists several cycles in the order the server gave them", () => {
    const message = eventCycleMessage(
      {
        cycles: [
          ["a", "b"],
          ["a", "c", "d"],
        ],
        hopLimit: 8,
      },
      pt,
      "pt-BR",
    );

    expect(message).toContain("a → b → a e a → c → d → a");
  });

  it("says an agent is never started by its own event", () => {
    const message = eventCycleMessage(
      { cycles: [["dev"]], hopLimit: 8 },
      en,
      "en-US",
    );

    expect(message).toContain("dev → dev");
    expect(message).toMatch(/never started by its own event/);
  });

  it("says nothing when the publish closed no cycle", () => {
    expect(eventCycleMessage(undefined, pt, "pt-BR")).toBeUndefined();
  });
});
