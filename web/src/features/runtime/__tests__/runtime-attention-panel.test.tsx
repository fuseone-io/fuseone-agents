import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { runtimeAttention } from "@/features/runtime/runtime-attention";
import { RuntimeAttentionPanel } from "@/features/runtime/runtime-attention-panel";
import { setLocale } from "@/i18n";
import type { RuntimeHealth } from "@/lib/api/client";

test("shows the main runtime failure families with stable labels", () => {
  setLocale("en-US");
  render(
    <RuntimeAttentionPanel
      health={runtimeHealth({
        failures: [
          {
            code: "model_provider_overloaded",
            provider: "anthropic",
            status: 529,
            retryable: true,
            runs: 4,
            lastAt: "2026-08-24T12:00:00.000Z",
          },
        ],
        toolFailures: [
          {
            code: "mcp_personal_credential_missing",
            calls: 3,
            runs: 2,
            lastAt: "2026-08-24T12:01:00.000Z",
          },
        ],
        channelFailures: [
          {
            code: "channel_delivery_failed",
            attempts: 7,
            conversations: 2,
            scopeWide: false,
            runs: 2,
            firstAt: "2026-08-24T11:00:00.000Z",
            lastAt: "2026-08-24T12:02:00.000Z",
          },
        ],
      })}
    />,
  );

  expect(screen.getByText("Model provider overloaded")).toBeInTheDocument();
  expect(screen.getByText("Personal MCP credential is missing")).toBeInTheDocument();
  expect(screen.getByText("Channel delivery failed")).toBeInTheDocument();
  expect(screen.queryByText("model_provider_overloaded")).not.toBeInTheDocument();
  expect(screen.queryByText("mcp_personal_credential_missing")).not.toBeInTheDocument();
  expect(screen.queryByText("channel_delivery_failed")).not.toBeInTheDocument();
});

test("does not silently truncate the attention list", () => {
  setLocale("en-US");
  render(
    <RuntimeAttentionPanel
      health={runtimeHealth({
        toolFailures: [
          "mcp_personal_credential_missing",
          "mcp_personal_credential_invalid",
          "mcp_personal_credential_read_failed",
          "mcp_personal_credential_no_principal",
          "mcp_server_rate_limited",
          "unknown_server",
          "unknown_tool",
          "tool_error",
          "invoke_error",
        ].map((code, index) => ({
          code,
          calls: 9 - index,
          runs: 1,
          lastAt: "2026-08-24T12:00:00.000Z",
        })),
      })}
    />,
  );

  expect(screen.getByText("8 of 9")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Load more" })).toBeInTheDocument();
});

test("orders items without timestamps deterministically", () => {
  const items = runtimeAttention(
    runtimeHealth({
      queue: {
        ready: 0,
        leased: 0,
        backingOff: 5,
        expiredLeases: 0,
      },
      failures: [
        {
          code: "model_provider_overloaded",
          provider: "anthropic",
          status: 529,
          retryable: true,
          runs: 5,
          lastAt: "2026-08-24T12:00:00.000Z",
        },
      ],
    }),
  );

  expect(items.map((item) => item.id)).toEqual([
    "provider:model_provider_overloaded:anthropic:529",
    "queue:backing_off",
  ]);
});

test("shows dedupe contention as coordination rather than provider failure", () => {
  setLocale("en-US");
  const health = runtimeHealth({
    failures: [
      {
        code: "dedupe_in_flight",
        retryable: true,
        runs: 2,
        lastAt: "2026-08-24T12:00:00.000Z",
      },
    ],
  });

  expect(runtimeAttention(health).map(({ id, kind }) => ({ id, kind }))).toEqual([
    { id: "coordination:dedupe_in_flight", kind: "coordination" },
  ]);

  render(<RuntimeAttentionPanel health={health} />);

  expect(screen.getByText("Duplicate effect still in flight")).toBeInTheDocument();
  expect(screen.getByText(/2 run\(s\) waiting on another run's effect reservation/)).toBeInTheDocument();
  expect(screen.queryByText(/2 run\(s\) parked or failed on the provider/)).not.toBeInTheDocument();
});

function runtimeHealth(input: Partial<RuntimeHealth>): RuntimeHealth {
  return {
    byPhase: {},
    queue: {
      ready: 0,
      leased: 0,
      backingOff: 0,
      expiredLeases: 0,
    },
    failures: [],
    toolFailures: [],
    channelFailures: [],
    egressDenials: [],
    ...input,
  };
}
