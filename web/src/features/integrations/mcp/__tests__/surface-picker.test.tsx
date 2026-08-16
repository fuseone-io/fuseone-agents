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
        onClassify={vi.fn()}
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
        onClassify={vi.fn()}
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
        onClassify={vi.fn()}
      />,
    );

    await userEvent.click(screen.getByRole("checkbox"));
    expect(toggle).toHaveBeenCalledWith("lookup", true);
  });
});

describe("ruling on a tool from where it was found", () => {
  it("offers the ruling on the row, so the Curator does not change screens mid-decision", async () => {
    const classify = vi.fn();
    render(
      <SurfacePicker
        tools={[{ ...tool("crm.lookup"), effect: "unknown" }]}
        chosen={new Set(["lookup"])}
        onToggle={vi.fn()}
        onClassify={classify}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /julgar/i }));
    expect(classify).toHaveBeenCalledWith(expect.objectContaining({ toolId: "crm.lookup" }));
  });

  // The button says which act this is. Ruling on a tool nobody has judged and
  // changing a decision somebody already made are not the same thing, and one
  // label for both invites the second by accident.
  it("says whether this would be a first ruling or a change of one", () => {
    render(
      <SurfacePicker
        tools={[tool("crm.lookup")]}
        chosen={new Set(["lookup"])}
        onToggle={vi.fn()}
        onClassify={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /mudar a decisão/i })).toBeInTheDocument();
  });

  it("shows what the platform would say, and does not say it", () => {
    render(
      <SurfacePicker
        tools={[
          {
            ...tool("crm.delete_account"),
            effect: "unknown",
            suggested: { effect: "destructive", why: "Removes a customer." },
          },
        ]}
        chosen={new Set(["delete_account"])}
        onToggle={vi.fn()}
        onClassify={vi.fn()}
      />,
    );

    // The proposal is visible; the pill still says nobody has ruled.
    expect(screen.getByText(/plataforma chamaria/i)).toBeInTheDocument();
    expect(screen.getByText(/não classificad/i)).toBeInTheDocument();
  });
});
