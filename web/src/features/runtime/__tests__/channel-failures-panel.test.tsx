import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { ChannelFailuresPanel } from "@/features/runtime/channel-failures-panel";

test("names stable channel failure codes instead of showing raw codes", () => {
  render(
    <ChannelFailuresPanel
      failures={[
        {
          code: "channel_missing_scope",
          attempts: 49,
          conversations: 7,
          runs: 3,
          firstAt: "2026-08-23T11:00:00.000Z",
          lastAt: "2026-08-23T12:00:00.000Z",
        },
      ]}
    />,
  );

  expect(screen.getByText("Falta um escopo no app do canal")).toBeInTheDocument();
  expect(screen.queryByText("channel_missing_scope")).not.toBeInTheDocument();
  expect(screen.getByText("49")).toBeInTheDocument();
  expect(screen.getByText("7")).toBeInTheDocument();
  expect(screen.getByText("3")).toBeInTheDocument();
});
