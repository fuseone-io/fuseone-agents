import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AdminNav } from "@/features/admin/admin-nav";
import { visibleAdminTabGroups } from "@/features/admin/admin-tabs";
import type { Tool } from "@/features/admin/api";

const hooks = vi.hoisted(() => ({
  tools: [] as Tool[],
}));

vi.mock("@/features/admin/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/admin/api")>();
  return {
    ...actual,
    useTools: () => ({ data: { items: hooks.tools } }),
  };
});

function tool(toolId: string, effect: Tool["effect"]): Tool {
  return { toolId, server: "crm", effect, untrusted: false };
}

describe("administration navigation", () => {
  beforeEach(() => {
    hooks.tools = [];
  });

  it("renders grouped settings navigation with the waiting queue as the only badge", async () => {
    hooks.tools = [
      tool("crm.lookup", "unknown"),
      { ...tool("crm.changed", "write"), stale: true },
      tool("crm.read", "read"),
    ];
    const choose = vi.fn();

    render(
      <AdminNav
        groups={visibleAdminTabGroups(null)}
        value="tools"
        onValueChange={choose}
      />,
    );

    expect(screen.getByText("Atividade")).toBeInTheDocument();
    expect(screen.getByText("Plataforma")).toBeInTheDocument();
    expect(screen.getByText("Organização")).toBeInTheDocument();
    expect(screen.getAllByText("Pessoas").length).toBeGreaterThan(0);
    expect(screen.getByText("Limites")).toBeInTheDocument();

    const tools = screen.getByRole("tab", {
      name: /Ferramentas à espera/,
    });
    expect(tools).toHaveAttribute("aria-selected", "true");
    expect(tools.className).toContain("bg-surface-accent");
    expect(within(tools).getByText("2")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: "Trilha" }));

    expect(choose).toHaveBeenCalledWith("events");
  });
});
