import { describe, expect, it } from "vitest";
import { edgePorts, placeGraph, PER_ROW } from "@/features/runs/run-graph-layout";
import type { FlowNode } from "@/features/runs/run-graph";

const nodes = (n: number): FlowNode[] =>
  Array.from({ length: n }, (_, i) => ({
    id: `s${i}`,
    seq: i,
    kind: "tool" as const,
    title: `n${i}`,
    tone: "neutral" as const,
  }));

describe("placing a run on the canvas", () => {
  it("puts the same run in the same places every time", () => {
    // The diagram appears on approval screens and in audit records. One that
    // reshuffled between renders would mean the approver did not see what was
    // recorded (PRD FU-17).
    const once = placeGraph(nodes(9));
    const twice = placeGraph(nodes(9));
    expect(once).toEqual(twice);
  });

  it("wraps to the next row and comes back the other way", () => {
    const placed = placeGraph(nodes(PER_ROW + 2));
    const first = placed.slice(0, PER_ROW);
    const second = placed.slice(PER_ROW);

    expect(new Set(first.map((p) => p.y)).size).toBe(1);
    expect(second[0]!.y).toBeGreaterThan(first[0]!.y);
    // Serpentine: the second row reads right to left, so the eye follows the
    // run instead of jumping back across the canvas at every wrap.
    expect(second[1]!.x).toBeLessThan(second[0]!.x);
  });

  it("places a single node without leaving it off the canvas", () => {
    expect(placeGraph(nodes(1))).toEqual([{ id: "s0", x: 0, y: 0 }]);
  });
});

describe("where an edge leaves and enters", () => {
  it("leaves on the right when the next node is to the right", () => {
    expect(edgePorts({ id: "a", x: 0, y: 0 }, { id: "b", x: 264, y: 0 })).toEqual({
      source: "right",
      target: "left",
    });
  });

  it("leaves on the left when the row reads backwards", () => {
    // Anchored right-to-left regardless, the edge would exit the node, loop
    // around the outside of the canvas and come back — which is what it did.
    expect(edgePorts({ id: "a", x: 264, y: 0 }, { id: "b", x: 0, y: 0 })).toEqual({
      source: "left",
      target: "right",
    });
  });

  it("drops straight down at the turn of a row", () => {
    expect(edgePorts({ id: "a", x: 792, y: 0 }, { id: "b", x: 792, y: 128 })).toEqual({
      source: "bottom",
      target: "top",
    });
  });
});
