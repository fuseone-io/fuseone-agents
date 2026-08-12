import type { FlowNode } from "@/features/runs/run-graph";

/**
 * Where each node sits.
 *
 * Computed rather than laid out by elkjs, which web/CLAUDE.md names for the
 * diagram. A run is a chain — the ledger is a sequence and there is no branch
 * to route around — so a serpentine is both sufficient and *more* deterministic
 * than a layout engine, whose output can shift between versions. The rule
 * exists so nobody hand-rolls a solver for a real graph; when the ledger gains
 * a branch, elk is the answer and this goes away.
 *
 * Determinism is not cosmetic: the diagram appears on approval screens and in
 * audit records, so one that reshuffled between renders would mean the
 * approver did not see what was recorded (PRD FU-17).
 */

export const NODE_WIDTH = 208;
export const NODE_HEIGHT = 76;
export const PER_ROW = 4;

const GAP_X = 56;
const GAP_Y = 52;

export interface Placement {
  id: string;
  x: number;
  y: number;
}

export function placeGraph(nodes: FlowNode[]): Placement[] {
  return nodes.map((node, i) => {
    const row = Math.floor(i / PER_ROW);
    const column = i % PER_ROW;
    // Odd rows read right to left, so the eye follows the run instead of
    // jumping back across the canvas at every wrap.
    const placed = row % 2 === 0 ? column : PER_ROW - 1 - column;
    return {
      id: node.id,
      x: placed * (NODE_WIDTH + GAP_X),
      y: row * (NODE_HEIGHT + GAP_Y),
    };
  });
}

/** The sides an edge should leave and enter by.
 *
 *  A serpentine reverses every other row, so anchoring every edge left-to-right
 *  makes half of them exit a node, loop around the outside of the canvas and
 *  come back in. The turn between rows drops straight down. */
export function edgePorts(
  from: Placement,
  to: Placement,
): { source: Port; target: Port } {
  if (to.x > from.x) return { source: "right", target: "left" };
  if (to.x < from.x) return { source: "left", target: "right" };
  return { source: "bottom", target: "top" };
}

export type Port = "left" | "right" | "top" | "bottom";
