import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SurfacePicker } from "@/features/integrations/mcp/surface-picker";
import type { Tool } from "@/features/admin/api";

function tool(toolId: string, declaredBy?: string[]): Tool {
  return {
    toolId,
    server: "crm",
    effect: "read",
    untrusted: false,
    ...(declaredBy ? { declaredBy } : {}),
  };
}

describe("choosing what this installation brought in", () => {
  it("says who stops when a tool is left out, before the choice is saved", async () => {
    render(
      <SurfacePicker
        tools={[tool("crm.delete_account", ["triagem", "cobranca"])]}
        chosen={new Set(["delete_account"])}
        onToggle={vi.fn()}
      />,
    );

    // Nothing yet: it is still on the surface.
    expect(screen.queryByText(/triagem/)).not.toBeInTheDocument();
  });

  it("names the agents that would stop, not just how many", () => {
    render(
      <SurfacePicker
        tools={[tool("crm.delete_account", ["triagem", "cobranca"])]}
        chosen={new Set()}
        onToggle={vi.fn()}
      />,
    );
    expect(screen.getByText(/triagem/)).toBeInTheDocument();
    expect(screen.getByText(/cobranca/)).toBeInTheDocument();
  });

  it("toggles by the name the server uses, which is what a surface is stored by", async () => {
    const toggle = vi.fn();
    render(
      <SurfacePicker
        tools={[tool("crm.lookup")]}
        chosen={new Set()}
        onToggle={toggle}
      />,
    );

    await userEvent.click(screen.getByRole("checkbox"));
    expect(toggle).toHaveBeenCalledWith("lookup", true);
  });
});
