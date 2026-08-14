import { describe, expect, it } from "vitest";
import { STEP_CELL } from "@/features/agents/agent-canvas-layout";
import { indexAt, placeAt } from "@/features/runs/run-graph-layout";

/*
The picture is derived, never stored.

Which makes determinism a property somebody depends on rather than a nicety:
the diagram appears on approval screens and in audit records, so the same
sequence has to draw the same thing at any time on any machine (FU-17, FU-18).
And because a dropped card has to find its cell, the grid has to answer the
question backwards as well — from a point to an index — with the two halves
agreeing.
*/

describe("the grid an agent's stages wrap into", () => {
  it("places the same step in the same cell every time", () => {
    const first = placeAt(5, STEP_CELL);
    for (let i = 0; i < 50; i++) {
      expect(placeAt(5, STEP_CELL)).toEqual(first);
    }
  });

  it("wraps rows in alternating directions, so the eye follows the sequence", () => {
    // The fifth step opens the second row, and a serpentine opens it under
    // the fourth rather than jumping back across the canvas.
    expect(placeAt(3, STEP_CELL).x).toBe(placeAt(4, STEP_CELL).x);
    expect(placeAt(4, STEP_CELL).y).toBeGreaterThan(placeAt(3, STEP_CELL).y);
  });

  it("finds the cell a card was dropped in", () => {
    // The two halves have to agree, or a card dropped exactly where another
    // one sits would reorder to somewhere else entirely.
    for (const at of [0, 1, 3, 4, 6, 7]) {
      const { x, y } = placeAt(at, STEP_CELL);
      expect(indexAt(x, y, 8, STEP_CELL)).toBe(at);
    }
  });

  it("keeps a card dragged off the edge inside the sequence", () => {
    expect(indexAt(-900, -900, 4, STEP_CELL)).toBe(0);
    expect(indexAt(9000, 9000, 4, STEP_CELL)).toBe(3);
  });
});
