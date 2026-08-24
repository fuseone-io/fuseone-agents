import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { ToolFailuresPanel } from "@/features/runtime/tool-failures-panel";

test("names stable MCP failure codes instead of showing raw codes", () => {
  render(
    <ToolFailuresPanel
      failures={[
        {
          code: "mcp_personal_credential_missing",
          calls: 3,
          runs: 2,
          lastAt: "2026-08-23T12:00:00.000Z",
        },
      ]}
    />,
  );

  expect(screen.getByText("Falta a credencial MCP pessoal")).toBeInTheDocument();
  expect(screen.queryByText("mcp_personal_credential_missing")).not.toBeInTheDocument();
  expect(screen.getByText("3")).toBeInTheDocument();
  expect(screen.getByText("2")).toBeInTheDocument();
});
