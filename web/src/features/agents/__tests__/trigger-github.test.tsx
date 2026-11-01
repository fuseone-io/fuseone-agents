import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeAll, beforeEach, describe, expect, it } from "vitest";
import { TriggerRow } from "@/features/agents/trigger-row";
import { eventsFrom, incomplete } from "@/features/agents/trigger-kinds";
import { setLocale } from "@/i18n";
import type { AgentTrigger } from "@/lib/api/client";

/*
A webhook can say GitHub sends it, and which deliveries start a run.

GitHub signs each delivery instead of sending the secret, and sends every
event the hook subscribes to. Without the events a GitHub hook would start a
run per delivery, so the row calls itself unfinished until they are named.
*/

let last: AgentTrigger = { type: "webhook" };

function Harness({ initial }: { initial: AgentTrigger }) {
  const [trigger, setTrigger] = useState(initial);
  return (
    <TriggerRow
      trigger={trigger}
      onChange={(over) => {
        last = { ...trigger, ...over };
        setTrigger(last);
      }}
      onRemove={() => {}}
    />
  );
}

describe("a webhook that comes from GitHub", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  });
  beforeEach(() => setLocale("en-US"));

  it("is unfinished until it names an event", () => {
    const github = {
      type: "webhook",
      path: "gh/reviewer",
      from: "github",
    } as const;
    expect(incomplete(github)).toBe(true);
    expect(incomplete({ ...github, events: ["pull_request.opened"] })).toBe(
      false,
    );
  });

  it("reads the events as a comma-separated list", () => {
    expect(eventsFrom(" pull_request.opened, issues ,,")).toEqual([
      "pull_request.opened",
      "issues",
    ]);
  });

  it("takes GitHub as the source and the events typed", async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ type: "webhook", path: "gh/reviewer" }} />);

    await user.click(screen.getByRole("combobox", { name: "Sent by" }));
    await user.click(await screen.findByRole("option", { name: /GitHub/ }));
    expect(screen.getByText(/Name the events/)).toBeInTheDocument();

    await user.type(
      screen.getByRole("textbox", { name: "GitHub events" }),
      "pull_request.opened, issues",
    );

    expect(last).toMatchObject({
      from: "github",
      events: ["pull_request.opened", "issues"],
    });
    expect(screen.queryByText(/Name the events/)).not.toBeInTheDocument();
  });

  it("drops the events when the source goes back to any sender", async () => {
    const user = userEvent.setup();
    render(
      <Harness
        initial={{
          type: "webhook",
          path: "a",
          from: "github",
          events: ["push"],
        }}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Sent by" }));
    await user.click(await screen.findByRole("option", { name: /Any sender/ }));

    expect(last.from).toBeUndefined();
    expect(last.events).toBeUndefined();
  });
});
