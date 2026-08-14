import type { Edge, Node } from "@xyflow/react";
import { STEP_CELL } from "@/features/agents/agent-canvas-layout";
import { edgePorts, placeAt } from "@/features/runs/run-graph-layout";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The picture, built from the sequence and nothing else.
 *
 * A pure function on purpose: it is the whole reason the same version draws
 * the same diagram at any time on any machine (FU-17, FU-18). Anything that
 * read the clock, a stored position or the order a node was last touched
 * would put that guarantee in the hands of whoever changed it next.
 */
export function buildGraph(
  steps: AgentStep[],
  view: { selected?: number; draggable: boolean },
): { nodes: Node[]; edges: Edge[] } {
  const nodes = steps.map((step, at): Node => ({
    id: String(at),
    type: "step",
    position: placeAt(at, STEP_CELL),
    draggable: view.draggable,
    selected: at === view.selected,
    data: {
      name: step.name,
      reaches: step.reaches ?? [],
      stopsWhen: step.stopsWhen,
      index: at,
    },
  }));

  // One edge per adjacency: the specification is a sequence and has no branch,
  // so an edge here means "then", never "if".
  const edges = steps.slice(1).map((_, at): Edge => {
    const ports = edgePorts(placeAt(at, STEP_CELL), placeAt(at + 1, STEP_CELL));
    return {
      id: `${at}-${at + 1}`,
      source: String(at),
      target: String(at + 1),
      sourceHandle: `s-${ports.source}`,
      targetHandle: `t-${ports.target}`,
      type: "smoothstep",
      // No arrow markers, per the handoff: within a sequence the direction is
      // the reading order, and an arrowhead on every edge is noise.
      style: { stroke: "var(--primary)", strokeWidth: 1.25 },
    };
  });

  return { nodes, edges };
}
