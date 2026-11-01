import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { EventDispatchPanel } from "@/features/runtime/event-dispatch-panel";
import { runtimeAttention } from "@/features/runtime/runtime-attention";
import { setLocale } from "@/i18n";
import type { RuntimeEventDispatch, RuntimeHealth } from "@/lib/api/client";

function dispatch(input: Partial<RuntimeEventDispatch>): RuntimeEventDispatch {
  return {
    status: "ok",
    ready: 0,
    heldByXmin: 0,
    countCapped: false,
    countCap: 1000,
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

  const note = screen.getByText(/cannot see since when/);
  // The grant that would reveal it is cluster-wide; the panel must not
  // present it as a harmless fix.
  expect(note).toHaveTextContent(/cluster-wide/);
  expect(note).toHaveTextContent(/expected/);
  expect(screen.queryByText(/has been open for/)).not.toBeInTheDocument();
});

test("a count stopped at the cap reads as a lower bound", () => {
  setLocale("en-US");
  render(
    <EventDispatchPanel
      dispatch={dispatch({ heldByXmin: 1000, countCapped: true, lagSeconds: 900 })}
    />,
  );

  expect(screen.getByText("1000+")).toBeInTheDocument();
  expect(screen.getByText(/or once 1000 runs are waiting/)).toBeInTheDocument();
  expect(screen.queryByText("1000")).not.toBeInTheDocument();
});

test("a caught-up dispatcher is healthy", () => {
  setLocale("en-US");
  render(<EventDispatchPanel dispatch={dispatch({})} />);

  expect(screen.getByText("Healthy")).toBeInTheDocument();
  expect(screen.getByText(/Caught up/)).toBeInTheDocument();
});

test("a degraded dispatcher needs attention, a healthy one does not", () => {
  const base: RuntimeHealth = {
    byPhase: {},
    queue: { ready: 0, leased: 0, backingOff: 0, expiredLeases: 0 },
    failures: [],
    toolFailures: [],
    channelFailures: [],
    egressDenials: [],
  };

  const degraded = runtimeAttention({
    ...base,
    eventDispatch: dispatch({ status: "degraded", ready: 1, heldByXmin: 4, lagSeconds: 400 }),
  });
  expect(degraded.map(({ id, count }) => ({ id, count }))).toEqual([
    { id: "dispatch:event_dispatch_degraded", count: 5 },
  ]);
  expect(runtimeAttention({ ...base, eventDispatch: dispatch({ ready: 3 }) })).toEqual([]);
});
