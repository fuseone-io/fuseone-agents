import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { AgentTriggersSection } from "@/features/agents/agent-triggers-section";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";

let draft: AgentDefinition;

function Harness({ initial }: { initial: AgentDefinition }) {
  const [current, setCurrent] = useState(initial);
  return (
    <AgentTriggersSection
      draft={current}
      patch={(over) => {
        draft = { ...current, ...over };
        setCurrent(draft);
      }}
    />
  );
}

const twoGitHubRows = {
  triggers: [
    { type: "webhook", path: "gh/a", from: "github", events: ["push"] },
    { type: "webhook", path: "gh/b", from: "github", events: ["issues"] },
  ],
} as AgentDefinition;

/*
Removing a row leaves the next one showing its own events.

The rows are keyed by position, so removing the first hands its component to
the second. Events kept as typed text in that component showed the removed
row's list over the remaining one — and the next keystroke wrote it there.
*/
describe("two GitHub webhook rows", () => {
  beforeEach(() => setLocale("en-US"));

  it("shows the remaining row's events after the first is removed", async () => {
    const user = userEvent.setup();
    render(<Harness initial={twoGitHubRows} />);

    await user.click(screen.getAllByRole("button", { name: "Remove" }).at(0)!);

    expect(screen.getByRole("textbox", { name: "GitHub events" })).toHaveValue(
      "issues",
    );
  });

  it("edits the remaining row, not the removed one", async () => {
    const user = userEvent.setup();
    render(<Harness initial={twoGitHubRows} />);

    await user.click(screen.getAllByRole("button", { name: "Remove" }).at(0)!);
    await user.type(
      screen.getByRole("textbox", { name: "GitHub events" }),
      ", pull_request",
    );

    expect(draft.triggers).toEqual([
      {
        type: "webhook",
        path: "gh/b",
        from: "github",
        events: ["issues", "pull_request"],
      },
    ]);
  });
});
