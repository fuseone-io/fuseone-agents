import { useMemo } from "react";
import { useTheme } from "next-themes";
import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { FlowNode } from "@/features/runs/flow-node";
import { buildGraph } from "@/features/runs/run-graph";
import { edgePorts, placeGraph } from "@/features/runs/run-graph-layout";
import type { Step } from "@/lib/api/client";

const NODE_TYPES = { step: FlowNode };

/**
 * The run, drawn.
 *
 * Read-only and generated: there is nothing to drag, because the picture is of
 * something that already happened (PRD N5). Nodes and edges are built on every
 * render and stored nowhere — the ledger is the record and this is a
 * projection of it (PRD FU-18).
 */
export function RunDiagram({
  steps,
  onSelect,
}: {
  steps: Step[];
  onSelect?: (seq: number) => void;
}) {
  // XYFlow ships its own chrome with its own palette. colorMode is how it is
  // told which one, and "system" is a value it understands — hand-styling its
  // controls with our tokens would fight the library on every upgrade.
  const { resolvedTheme } = useTheme();

  const { nodes, edges } = useMemo(() => {
    const graph = buildGraph(steps);
    const placed = new Map(placeGraph(graph.nodes).map((p) => [p.id, p]));

    return {
      nodes: graph.nodes.map((node): Node => ({
        id: node.id,
        type: "step",
        position: {
          x: placed.get(node.id)?.x ?? 0,
          y: placed.get(node.id)?.y ?? 0,
        },
        data: { ...node },
        draggable: false,
      })),
      edges: graph.edges.map((edge): Edge => {
        const from = placed.get(edge.from);
        const to = placed.get(edge.to);
        const ports =
          from && to
            ? edgePorts(from, to)
            : { source: "right", target: "left" };
        return {
          id: `${edge.from}-${edge.to}`,
          source: edge.from,
          target: edge.to,
          sourceHandle: `${ports.source}-out`,
          targetHandle: ports.target,
        };
      }),
    };
  }, [steps]);

  return (
    <div className="h-[420px] w-full">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={NODE_TYPES}
        colorMode={resolvedTheme === "dark" ? "dark" : "light"}
        fitView
        // Nothing here edits anything. Panning and zooming stay, because a run
        // of forty steps does not fit a panel at a readable size.
        nodesConnectable={false}
        elementsSelectable={!!onSelect}
        proOptions={{ hideAttribution: true }}
        onNodeClick={(_, node) => onSelect?.(Number(String(node.id).slice(1)))}
      >
        <Background gap={18} size={1} className="text-border-subtle" />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
