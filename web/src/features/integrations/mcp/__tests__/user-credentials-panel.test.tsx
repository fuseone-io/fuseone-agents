import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UserCredentialsPanel } from "@/features/integrations/mcp/user-credentials-panel";
import type {
  MCPServer,
  MCPUserCredential,
} from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

const api = vi.hoisted(() => ({
  putAsync: vi.fn(),
  deleteAsync: vi.fn(),
}));

vi.mock("@/features/integrations/api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/integrations/api")>();
  return {
    ...actual,
    usePutMCPUserCredential: () => ({
      mutateAsync: api.putAsync,
      isPending: false,
    }),
    useDeleteMCPUserCredential: () => ({
      mutateAsync: api.deleteAsync,
      isPending: false,
    }),
  };
});

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const datadog: ServerRecipe = {
  server: "datadog",
  title: "Datadog",
  category: "operations",
  publisher: "Datadog",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "published",
  configRequirements: ["credential"],
  authModes: [
    {
      type: "headers",
      principal: "service",
      label: "API and application key headers",
      headers: ["DD_API_KEY", "DD_APPLICATION_KEY"],
    },
  ],
  transport: "http",
  url: "https://mcp.datadoghq.com/api/unstable/mcp-server/mcp",
};

const outline: ServerRecipe = {
  server: "outline",
  title: "Outline",
  category: "knowledge",
  publisher: "Outline",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "published",
  configRequirements: ["credential"],
  authModes: [
    { type: "oauth2", principal: "user", label: "Outline OAuth" },
    {
      type: "bearer",
      principal: "user",
      label: "Outline API key",
      header: "Authorization",
      prefix: "Bearer",
    },
  ],
  transport: "http",
  url: "https://example.getoutline.com/mcp",
};

function remote(name: string): MCPServer {
  return {
    name,
    transport: "http",
    url: `https://${name}.example/mcp`,
    enabled: true,
  };
}

function local(name: string): MCPServer {
  return {
    name,
    transport: "stdio",
    command: "toolbox",
    enabled: true,
  };
}

function credential(server: string, overrides: Partial<MCPUserCredential> = {}) {
  return {
    server,
    hasCredential: true,
    hasHeaders: false,
    hasOAuth: false,
    ...overrides,
  };
}

function open({
  servers = [remote("datadog")],
  recipes = [datadog],
  credentials = [],
}: {
  servers?: MCPServer[];
  recipes?: ServerRecipe[];
  credentials?: MCPUserCredential[];
} = {}) {
  return render(
    <UserCredentialsPanel
      servers={servers}
      recipes={recipes}
      credentials={credentials}
      isLoading={false}
      error={null}
      onRetry={vi.fn()}
    />,
  );
}

describe("personal MCP credentials", () => {
  beforeEach(() => {
    api.putAsync.mockReset();
    api.putAsync.mockResolvedValue(undefined);
    api.deleteAsync.mockReset();
    api.deleteAsync.mockResolvedValue(undefined);
  });

  it("stores the selected user's multi-header credential without changing the server", async () => {
    open();

    await userEvent.type(screen.getByLabelText("DD_API_KEY"), "api_secret");
    await userEvent.type(
      screen.getByLabelText("DD_APPLICATION_KEY"),
      "app_secret",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Salvar minha credencial" }),
    );

    expect(api.putAsync).toHaveBeenCalledWith({
      name: "datadog",
      headers: {
        DD_API_KEY: "api_secret",
        DD_APPLICATION_KEY: "app_secret",
      },
    });
  });

  it("does not choose between two credential shapes for the user", async () => {
    open({
      recipes: [
        {
          ...datadog,
          authModes: [
            { type: "bearer", principal: "service", label: "Access token" },
            {
              type: "headers",
              principal: "service",
              label: "Header token",
              headers: ["Api-Key"],
            },
          ],
        },
      ],
    });

    await userEvent.type(screen.getByLabelText(/access token/i), "bearer");
    await userEvent.type(screen.getByLabelText("Api-Key"), "api_secret");

    expect(
      screen.getByText(/preencha só uma forma de credencial/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Salvar minha credencial" }),
    ).toBeDisabled();
    expect(api.putAsync).not.toHaveBeenCalled();
  });

  it("removes the signed-in user's credential without touching shared credentials", async () => {
    open({
      servers: [remote("google-workspace")],
      recipes: [
        {
          ...datadog,
          server: "google-workspace",
          title: "Google Workspace",
          authModes: [
            { type: "oauth2", principal: "user", label: "Google OAuth" },
          ],
        },
      ],
      credentials: [
        credential("google-workspace", {
          hasCredential: true,
          hasOAuth: true,
        }),
      ],
    });

    await userEvent.click(
      screen.getByRole("button", { name: /remover grant oauth/i }),
    );

    expect(api.deleteAsync).toHaveBeenCalledWith("google-workspace");
    expect(api.putAsync).not.toHaveBeenCalled();
  });

  it("does not describe a user-only server credential as a runtime fallback", () => {
    open({
      servers: [
        remote("outline"),
      ],
      recipes: [outline],
      credentials: [],
    });

    expect(
      screen.getByText(/credencial pessoal para chamadas de ferramenta/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/execuções sem a sua própria credencial caem nela/i),
    ).not.toBeInTheDocument();
  });

  it("does not offer personal credentials for local processes", () => {
    open({ servers: [local("filesystem")], recipes: [], credentials: [] });

    expect(screen.getByText(/nenhum mcp remoto configurado/i)).toBeInTheDocument();
  });
});
