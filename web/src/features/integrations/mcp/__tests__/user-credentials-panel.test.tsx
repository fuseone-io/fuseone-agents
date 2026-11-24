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
  requiresPersonalCredential: false,
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
  requiresPersonalCredential: true,
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

  it("asks which shape the user's credential takes, and sends only that one", async () => {
    open({
      recipes: [
        {
          ...datadog,
          authModes: [
            { type: "bearer", principal: "service", label: "Access token" },
            { type: "headers", principal: "service", label: "Header token", headers: ["Api-Key"] },
          ],
        },
      ],
    });

    await userEvent.type(screen.getByLabelText(/access token/i), "bearer");
    await userEvent.click(screen.getByRole("radio", { name: /^Cabeçalhos/ }));
    expect(screen.queryByLabelText(/access token/i)).not.toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Api-Key"), "api_secret");
    await userEvent.click(screen.getByRole("button", { name: "Salvar minha credencial" }));

    expect(api.putAsync).toHaveBeenCalledWith({ name: "datadog", headers: { "Api-Key": "api_secret" } });
  });

  it("starts the user's shape empty after switching away from it and back", async () => {
    open({
      recipes: [
        {
          ...datadog,
          authModes: [
            { type: "bearer", principal: "service", label: "Access token" },
            { type: "headers", principal: "service", label: "Header token", headers: ["Api-Key"] },
          ],
        },
      ],
    });

    await userEvent.type(screen.getByLabelText(/access token/i), "bearer");
    await userEvent.click(screen.getByRole("radio", { name: /^Cabeçalhos/ }));
    await userEvent.click(screen.getByRole("radio", { name: /^Token/ }));
    expect(screen.getByLabelText(/access token/i)).toHaveValue("");
  });

  it("follows the recipe when it arrives after the panel", () => {
    const oauthOnly = { ...datadog, authModes: [{ type: "oauth2" as const, principal: "user" as const }] };
    const panel = (recipes: ServerRecipe[]) => (
      <UserCredentialsPanel servers={[remote("datadog")]} recipes={recipes} credentials={[]}
        isLoading={false} error={null} onRetry={vi.fn()} />
    );
    const { rerender } = render(panel([]));
    rerender(panel([oauthOnly]));

    expect(screen.getByLabelText(/access token oauth/i)).toBeInTheDocument();
  });

  it("lets a stored personal token be revoked where the recipe now offers only OAuth", async () => {
    open({
      recipes: [{ ...datadog, authModes: [{ type: "oauth2", principal: "user" }] }],
      credentials: [credential("datadog")],
    });

    await userEvent.click(screen.getByRole("button", { name: "Remover a credencial guardada" }));
    expect(api.deleteAsync).toHaveBeenCalledWith("datadog");
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
