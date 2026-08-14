import { useEffect, useMemo } from "react";
import { useTheme } from "next-themes";
import {
  Background,
  Controls,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Edge,
  type Node,
  type NodeChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { StepNode } from "@/features/agents/step-node";
import { STEP_CELL } from "@/features/agents/agent-canvas-layout";
import { edgePorts, indexAt, placeAt } from "@/features/runs/run-graph-layout";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

const NODE_TYPES = { step: StepNode };

/**
 * The stages of an agent, drawn.
 *
 * Nothing about this picture is stored — no position, no node identifier, no
 * edge handle. The layout is recomputed from the sequence on every render, so
 * the same version draws the same diagram at any time on any machine, which is
 * what makes it citable on an approval screen and in an audit record (FU-17,
 * FU-18). A saved coordinate would be a second artefact that can disagree with
 * the text, and it would disagree silently.
 *
 * Which is why dragging reorders rather than moves. There is nowhere to keep
 * "the author put this box here", so a card dropped in another cell changes
 * the sequence — the fact — and the grid re-derives from it (NT-007 §2.1).
 */
export function AgentCanvas(props: {
  steps: AgentStep[];
  onReorder?: (from: number, to: number) => void;
  onSelect?: (at: number) => void;
}) {
  // The provider is what lets the canvas refit itself when the sequence
  // changes, which is the whole of its state — nothing about the picture
  // outlives the render.
  return (
    <ReactFlowProvider>
      <Canvas {...props} />
    </ReactFlowProvider>
  );
}

function Canvas({
  steps,
  onReorder,
  onSelect,
}: {
  steps: AgentStep[];
  onReorder?: (from: number, to: number) => void;
  onSelect?: (at: number) => void;
}) {
  // XYFlow ships its own chrome with its own palette, and colorMode is how it
  // is told which. Hand-styling its controls would fight the library on every
  // upgrade.
  const { resolvedTheme } = useTheme();

  const { nodes, edges } = useMemo(() => {
    const nodes = steps.map((step, at): Node => ({
      id: String(at),
      type: "step",
      position: placeAt(at, STEP_CELL),
      draggable: onReorder !== undefined,
      data: {
        name: step.name,
        reaches: step.reaches ?? [],
        stopsWhen: step.stopsWhen,
        index: at,
      },
    }));

    // One edge per adjacency: the specification is a sequence and has no
    // branch, so an edge here means "then", never "if".
    const edges = steps.slice(1).map((_, at): Edge => {
      const ports = edgePorts(placeAt(at, STEP_CELL), placeAt(at + 1, STEP_CELL));
      return {
        id: `${at}-${at + 1}`,
        source: String(at),
        target: String(at + 1),
        sourceHandle: `s-${ports.source}`,
        targetHandle: `t-${ports.target}`,
        type: "smoothstep",
        // No arrow markers, per the handoff: within a sequence the direction
        // is the reading order, and an arrowhead on every edge is noise.
        style: { stroke: "var(--primary)", strokeWidth: 1.25 },
      };
    });

    return { nodes, edges };
  }, [steps, onReorder]);

  // Refit whenever the sequence changes. fitView alone runs at mount, so a
  // step added afterwards fell outside the viewport and read as a canvas that
  // had lost it.
  const flow = useReactFlow();
  useEffect(() => {
    void flow.fitView({ padding: 0.2, duration: 0 });
  }, [flow, steps.length]);

  const settle = (changes: NodeChange[]) => {
    for (const change of changes) {
      if (change.type !== "position" || change.dragging !== false) continue;
      const from = Number(change.id);
      const at = nodes.find((node) => node.id === change.id);
      if (!at || !change.position) continue;
      const to = indexAt(change.position.x, change.position.y, steps.length, STEP_CELL);
      if (to !== from) onReorder?.(from, to);
    }
  };

  return (
    <div className="h-[420px] w-full overflow-hidden rounded-lg border border-border bg-background">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={NODE_TYPES}
        colorMode={resolvedTheme === "dark" ? "dark" : "light"}
        onNodesChange={settle}
        onNodeClick={(_, node) => onSelect?.(Number(node.id))}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        proOptions={{ hideAttribution: true }}
      >
        <Background gap={16} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
