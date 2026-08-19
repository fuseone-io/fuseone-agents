import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Waiting } from "@/features/admin/tools-panel";
import { waitingFor } from "@/features/admin/waiting-tools";
import type { Tool } from "@/features/admin/api";

function tool(toolId: string, effect: Tool["effect"]): Tool {
  return { toolId, server: "crm", effect, untrusted: false };
}

describe("what the tools panel says about tools nobody has ruled on", () => {
  it("says they are refused, because that is what the Gate does with them", () => {
    render(<Waiting tools={[tool("crm.lookup", "unknown"), tool("crm.note", "read")]} />);

    // The panel used to say tools "arrive as reads" — true once, and a screen
    // still describing the behaviour it had before sends somebody to look for
    // the fault somewhere else.
    expect(screen.getByText(/recusada/i)).toBeInTheDocument();
    expect(screen.getByText(/\b1\b/)).toBeInTheDocument();
  });

  it("counts only the unruled ones, so the number is the work left", () => {
    render(
      <Waiting
        tools={[
          tool("a", "unknown"),
          tool("b", "unknown"),
          tool("c", "destructive"),
        ]}
      />,
    );
    expect(screen.getByText(/\b2\b/)).toBeInTheDocument();
  });

  it("says what arrives, when nothing is waiting", () => {
    render(<Waiting tools={[tool("crm.lookup", "read")]} />);
    expect(screen.getByText(/sem classificação/i)).toBeInTheDocument();
  });
});

describe("what it says about a ruling the tool outgrew", () => {
  it("counts it as work, because the Gate refuses it exactly as it refuses an unruled one", () => {
    render(
      <Waiting
        tools={[
          { ...tool("crm.lookup", "read"), stale: true },
          tool("crm.note", "read"),
        ]}
      />,
    );
    expect(screen.getByText(/recusada/i)).toBeInTheDocument();
    expect(screen.getByText(/\b1\b/)).toBeInTheDocument();
  });
});

describe("what the queue holds", () => {
  /*
   * Not a second catalogue. Every tool with its ruling lives on the server
   * that offers it, where the surrounding facts are; listing them all again
   * here was the same rows in two places.
   *
   * What it answers is the question no per-server page can: across the whole
   * installation, what is waiting. Ten servers is ten visits to find out
   * there is nothing to do.
   */
  it("keeps only what the Gate is refusing", () => {
    const held = waitingFor([
      tool("crm.lookup", "read"),
      tool("crm.new", "unknown"),
      { ...tool("crm.changed", "write"), stale: true },
    ]);
    expect(held.map((one) => one.toolId)).toEqual(["crm.new", "crm.changed"]);
  });
});
