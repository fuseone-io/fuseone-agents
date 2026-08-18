import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, beforeEach, vi } from "vitest";
import { ConnectionPanel } from "@/features/integrations/mcp/connection-panel";
import type { MCPServer } from "@/features/integrations/api";

const api = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
}));

vi.mock("@/features/integrations/api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/integrations/api")>();
  return {
    ...actual,
    usePutMCPServer: () => ({ mutateAsync: api.mutateAsync, isPending: false }),
  };
});

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function remote(overrides: Partial<MCPServer> = {}): MCPServer {
  return {
    name: "google-sheets",
    transport: "http",
    url: "https://mcp.example.com/google",
    enabled: true,
    ...overrides,
  };
}

describe("the MCP connection panel", () => {
  beforeEach(() => {
    api.mutateAsync.mockReset();
    api.mutateAsync.mockResolvedValue(undefined);
  });

  it("saves a manual OAuth grant as oauth rather than a bearer token", async () => {
    render(<ConnectionPanel server={remote()} />);

    await userEvent.type(screen.getByLabelText(/access token oauth/i), "access");
    await userEvent.type(screen.getByLabelText(/refresh token oauth/i), "refresh");
    await userEvent.type(
      screen.getByLabelText(/url de token oauth/i),
      "https://issuer.example/token",
    );
    await userEvent.type(screen.getByLabelText(/client id oauth/i), "client");
    await userEvent.type(screen.getByLabelText(/client secret oauth/i), "secret");
    await userEvent.type(screen.getByLabelText(/escopos oauth/i), "sheets.readonly");

    await userEvent.click(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        token: undefined,
        oauth: expect.objectContaining({
          accessToken: "access",
          refreshToken: "refresh",
          tokenURL: "https://issuer.example/token",
          clientID: "client",
          clientSecret: "secret",
          scopes: ["sheets.readonly"],
        }),
      }),
    );
  });

  it("does not choose between bearer and OAuth on behalf of the operator", async () => {
    render(<ConnectionPanel server={remote()} />);

    await userEvent.type(screen.getByLabelText(/^token$/i), "bearer");
    await userEvent.type(screen.getByLabelText(/access token oauth/i), "access");

    expect(
      screen.getByText(/preencha bearer token ou grant oauth/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    ).toBeDisabled();
  });

  it("revokes a stored OAuth grant with an empty oauth object", async () => {
    render(<ConnectionPanel server={remote({ hasSecret: true, hasOAuth: true })} />);

    await userEvent.click(
      screen.getByRole("button", { name: /remover grant oauth/i }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ oauth: {}, token: undefined }),
    );
  });
});
