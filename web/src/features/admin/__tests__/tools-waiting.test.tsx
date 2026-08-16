import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Waiting } from "@/features/admin/tools-panel";
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
