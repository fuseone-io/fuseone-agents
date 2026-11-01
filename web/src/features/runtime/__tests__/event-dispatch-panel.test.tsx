import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { EventDispatchPanel } from "@/features/runtime/event-dispatch-panel";
import { setLocale } from "@/i18n";
import type { RuntimeEventDispatch } from "@/lib/api/client";

function dispatch(input: Partial<RuntimeEventDispatch>): RuntimeEventDispatch {
  return {
    status: "ok",
    ready: 0,
    heldByXmin: 0,
    lagSeconds: 0,
    xactOpen: false,
    xactAgeKnown: true,
    degradedAfterSeconds: 300,
    ...input,
  };
}

test("a backlog held by an open transaction degrades and names its age", () => {
  setLocale("en-US");
  render(
    <EventDispatchPanel
      dispatch={dispatch({
        status: "degraded",
        ready: 2,
        heldByXmin: 7,
        lagSeconds: 420,
        oldestFinishedAt: "2026-11-01T11:53:00.000Z",
        xactOpen: true,
        xactAgeSeconds: 720,
      })}
    />,
  );

  expect(screen.getByText("Degraded")).toBeInTheDocument();
  expect(screen.getByText("2")).toBeInTheDocument();
  expect(screen.getByText("7")).toBeInTheDocument();
  expect(screen.getByText(/The oldest waiting run finished 7m 0s ago/)).toBeInTheDocument();
  expect(screen.getByText(/has been open for 12m 0s/)).toBeInTheDocument();
});

test("a holder the database role cannot see says so instead of showing an age", () => {
  setLocale("en-US");
  render(
    <EventDispatchPanel
      dispatch={dispatch({
        heldByXmin: 1,
        lagSeconds: 30,
        xactOpen: true,
        xactAgeKnown: false,
      })}
    />,
  );

  expect(screen.getByText(/cannot see since when/)).toBeInTheDocument();
  expect(screen.queryByText(/has been open for/)).not.toBeInTheDocument();
});

test("a caught-up dispatcher is healthy", () => {
  setLocale("en-US");
  render(<EventDispatchPanel dispatch={dispatch({})} />);

  expect(screen.getByText("Healthy")).toBeInTheDocument();
  expect(screen.getByText(/Caught up/)).toBeInTheDocument();
});
