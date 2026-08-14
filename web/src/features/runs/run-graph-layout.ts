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

/** The cell a serpentine wraps into. */
export interface Cell {
  width: number;
  height: number;
  gapX: number;
  gapY: number;
  perRow: number;
}

const RUN_CELL: Cell = {
  width: NODE_WIDTH,
  height: NODE_HEIGHT,
  gapX: GAP_X,
  gapY: GAP_Y,
  perRow: PER_ROW,
};

/**
 * Where the nth node sits, on its own.
 *
 * Exported because a canvas that lets somebody move a card has to answer the
 * question backwards — which cell did they drop it nearest — and two
 * implementations of one grid would drift the first time either changed.
 */
export function placeAt(index: number, cell: Cell = RUN_CELL): Omit<Placement, "id"> {
  const row = Math.floor(index / cell.perRow);
  const column = index % cell.perRow;
  // Odd rows read right to left, so the eye follows the run instead of
  // jumping back across the canvas at every wrap.
  const placed = row % 2 === 0 ? column : cell.perRow - 1 - column;
  return {
    x: placed * (cell.width + cell.gapX),
    y: row * (cell.height + cell.gapY),
  };
}

/** Which cell a point falls in, which is how a dropped card finds its place. */
export function indexAt(x: number, y: number, count: number, cell: Cell = RUN_CELL): number {
  const row = Math.max(0, Math.round(y / (cell.height + cell.gapY)));
  const column = Math.max(
    0,
    Math.min(cell.perRow - 1, Math.round(x / (cell.width + cell.gapX))),
  );
  const placed = row % 2 === 0 ? column : cell.perRow - 1 - column;
  return Math.max(0, Math.min(count - 1, row * cell.perRow + placed));
}

export function placeGraph(nodes: FlowNode[], cell: Cell = RUN_CELL): Placement[] {
  return nodes.map((node, i) => ({ id: node.id, ...placeAt(i, cell) }));
}

/** The sides an edge should leave and enter by.
 *
 *  A serpentine reverses every other row, so anchoring every edge left-to-right
 *  makes half of them exit a node, loop around the outside of the canvas and
 *  come back in. The turn between rows drops straight down. */
/**
 * Which side each end of an edge leaves and arrives on.
 *
 * A point rather than a Placement: the agent canvas asks about cells it has
 * not built nodes for yet, and an identifier it would have to invent to ask
 * the question is an identifier that means nothing.
 */
export function edgePorts(
  from: Point,
  to: Point,
): { source: Port; target: Port } {
  if (to.x > from.x) return { source: "right", target: "left" };
  if (to.x < from.x) return { source: "left", target: "right" };
  return { source: "bottom", target: "top" };
}

export type Port = "left" | "right" | "top" | "bottom";

/** Where something sits, without needing to be anything. */
export interface Point {
  x: number;
  y: number;
}
