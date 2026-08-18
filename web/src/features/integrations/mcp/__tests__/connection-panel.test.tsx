import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, beforeEach, vi } from "vitest";
import { ConnectionPanel } from "@/features/integrations/mcp/connection-panel";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

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

function recipe(
  authModes: NonNullable<ServerRecipe["authModes"]>,
): ServerRecipe {
  return {
    server: "google-sheets",
    title: "Google Sheets",
    category: "data",
    publisher: "Google",
    docsFrom: "publisher",
    provenance: "documentation",
    status: "published",
    configRequirements: ["credential"],
    authModes,
    transport: "http",
    url: "https://mcp.example.com/google",
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

    await userEvent.type(screen.getByLabelText(/token bearer/i), "bearer");
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

  it("does not render a bearer token field for an OAuth-only recipe", async () => {
    const { container } = render(
      <ConnectionPanel
        server={remote()}
        recipe={recipe([{ type: "oauth2", principal: "user", label: "Google OAuth" }])}
      />,
    );

    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/access token oauth/i)).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText(/access token oauth/i), "access");
    await userEvent.click(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        token: undefined,
        oauth: expect.objectContaining({ accessToken: "access" }),
      }),
    );
  });

  it("does not pretend custom headers are a bearer token", () => {
    const { container } = render(
      <ConnectionPanel
        server={remote({ name: "newrelic" })}
        recipe={recipe([
          {
            type: "headers",
            principal: "service",
            label: "New Relic API key",
            header: "Api-Key",
          },
        ])}
      />,
    );

    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    expect(
      screen.getByText(/espera New Relic API key/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    ).toBeDisabled();
  });
});
