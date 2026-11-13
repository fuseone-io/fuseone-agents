import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ToolsPanel, Waiting } from "@/features/admin/tools-panel";
import { waitingFor } from "@/features/admin/waiting-tools";
import type { Tool } from "@/features/admin/api";
import { json } from "@/test/revised-routes";

afterEach(() => vi.unstubAllGlobals());

/** The catalogue at the network; no session, so every door is open. */
function stubCatalogue(items: Tool[]) {
  vi.stubGlobal("fetch", async (input: Request | string) => {
    const url = typeof input === "string" ? input : input.url;
    return url.includes("/admin/tools") ? json({ items }) : json({}, 404);
  });
}

function tool(toolId: string, effect: Tool["effect"]): Tool {
  return { toolId, server: "crm", effect, untrusted: false };
}

describe("what the tools panel says about tools nobody has ruled on", () => {
  it("says they are refused, because that is what the Gate does with them", () => {
    render(
      <Waiting
        tools={[tool("crm.lookup", "unknown"), tool("crm.note", "read")]}
      />,
    );

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

describe("the tools waiting table", () => {
  it("keeps the classification action visible when a tool description is long", async () => {
    const description =
      "Add a comment and/or reaction to a specific issue or issue comment in a GitHub repository. Use this tool with pull requests as well and only when the requester really asked for it.";
    stubCatalogue([
      { ...tool("github.add_issue_comment", "unknown"), server: "github", description },
    ]);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <ToolsPanel />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const table = await screen.findByRole("table");
    expect(table).toHaveClass("min-w-[1040px]", "table-fixed");
    const columns = table.querySelectorAll("col");
    expect([...columns].map((column) => column.className)).toEqual([
      "w-[400px]",
      "w-[160px]",
      "w-[112px]",
      "w-[96px]",
      "w-[152px]",
      "w-[120px]",
    ]);
    const text = screen.getByText(description);
    expect(text).toHaveClass("truncate");
    expect(text.closest("td")).toHaveClass("max-w-0");
    const row = text.closest("tr");
    expect(row).not.toBeNull();
    expect(screen.getAllByRole("columnheader")).toHaveLength(columns.length);
    expect((row as HTMLTableRowElement).querySelectorAll("td")).toHaveLength(
      columns.length,
    );
    expect(
      within(row as HTMLTableRowElement).getByRole("button", {
        name: /classificar/i,
      }),
    ).toBeInTheDocument();
  });
});
