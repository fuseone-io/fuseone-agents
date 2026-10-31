import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ConnectionPanel } from "@/features/integrations/mcp/connection-panel";
import { AvailableServersPanel } from "@/features/integrations/mcp/available-servers-panel";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

const api = vi.hoisted(() => ({ mutateAsync: vi.fn(), probeAsync: vi.fn() }));

vi.mock("@/features/integrations/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/integrations/api")>();
  return {
    ...actual,
    usePutMCPServer: () => ({ mutateAsync: api.mutateAsync, isPending: false }),
    useProbeMCPServer: () => ({ mutateAsync: api.probeAsync, isPending: false }),
  };
});

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const KEY = "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEAsecret\n-----END RSA PRIVATE KEY-----";

const github: ServerRecipe = {
  server: "github", title: "GitHub", category: "code", publisher: "GitHub",
  docsFrom: "publisher", provenance: "documentation", status: "published",
  configRequirements: ["credential"], requiresPersonalCredential: false,
  transport: "http", url: "https://api.githubcopilot.com/mcp/",
  authModes: [
    { type: "bearer", principal: "user", label: "GitHub personal access token" },
    { type: "github_app", principal: "installation", label: "GitHub App" },
  ],
};

function server(overrides: Partial<MCPServer> = {}): MCPServer {
  return {
    name: "github", transport: "http", url: "https://api.githubcopilot.com/mcp/",
    enabled: true, ...overrides,
  };
}

function renderCatalogue() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AvailableServersPanel servers={[]} recipes={[github]} isLoading={false}
          error={null} onRetry={vi.fn()} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function sent() {
  return api.mutateAsync.mock.calls.at(-1)?.[0];
}

describe("the GitHub App credential", () => {
  beforeEach(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    api.mutateAsync.mockReset();
    api.mutateAsync.mockResolvedValue(undefined);
  });

  it("saves the App, installation, pasted key and repositories", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={server()} recipe={github} />);

    await user.type(screen.getByLabelText("ID do App"), "5550600");
    await user.type(screen.getByLabelText("ID da instalação"), "91");
    await user.click(screen.getByLabelText("Chave privada (PEM)"));
    await user.paste(KEY);
    await user.type(screen.getByLabelText("Repositórios (opcional)"), "fuseone-agents, site");
    await user.click(screen.getByRole("button", { name: "Salvar o GitHub App" }));

    expect(sent().githubApp).toEqual({
      appId: 5550600, installationId: 91, privateKey: KEY,
      repositories: ["fuseone-agents", "site"], apiBaseURL: undefined,
    });
  });

  it("reads the key from an uploaded file", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={server()} recipe={github} />);

    await user.type(screen.getByLabelText("ID do App"), "5550600");
    await user.type(screen.getByLabelText("ID da instalação"), "91");
    await user.upload(screen.getByLabelText("Enviar arquivo da chave"),
      new File([KEY], "app.private-key.pem", { type: "application/x-pem-file" }));
    expect(await screen.findByDisplayValue(/BEGIN RSA PRIVATE KEY/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Salvar o GitHub App" }));

    expect(sent().githubApp.privateKey).toBe(KEY);
  });

  it("shows a stored App as configured, never its key, and keeps it on a keyless edit", async () => {
    const user = userEvent.setup();
    const stored = { appId: 5550600, installationId: 91, hasPrivateKey: true };
    render(<ConnectionPanel server={server({ hasSecret: true, githubApp: stored })} recipe={github} />);

    expect(screen.getByText(/App 5550600, instalação 91/)).toBeInTheDocument();
    expect(screen.getByText(/Chave privada configurada/)).toBeInTheDocument();
    expect(screen.getByLabelText("Chave privada (PEM)")).toHaveValue("");
    await user.type(screen.getByLabelText("ID do App"), "5550600");
    await user.type(screen.getByLabelText("ID da instalação"), "92");
    await user.click(screen.getByRole("button", { name: "Salvar o GitHub App" }));

    expect(sent().githubApp).toMatchObject({ appId: 5550600, installationId: 92 });
    expect(sent().githubApp.privateKey).toBeUndefined();
  });

  it("removes a stored App with an empty object", async () => {
    const user = userEvent.setup();
    const stored = { appId: 5550600, installationId: 91, hasPrivateKey: true };
    render(<ConnectionPanel server={server({ hasSecret: true, githubApp: stored })} recipe={github} />);

    await user.click(screen.getByRole("button", { name: "Remover o GitHub App" }));

    expect(sent().githubApp).toEqual({});
  });

  it("does not save a new App without its key or with a non-numeric id", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={server()} recipe={github} />);

    await user.type(screen.getByLabelText("ID do App"), "factory-qa");
    await user.type(screen.getByLabelText("ID da instalação"), "91");

    expect(screen.getByRole("button", { name: "Salvar o GitHub App" })).toBeDisabled();
    expect(screen.getByText(/números inteiros/)).toBeInTheDocument();
  });

  it("is offered when connecting the GitHub recipe from the catalogue", async () => {
    const user = userEvent.setup();
    renderCatalogue();

    await user.click(screen.getByRole("button", { name: "Conectar GitHub" }));
    await user.type(screen.getByLabelText("ID do App"), "5550600");
    await user.type(screen.getByLabelText("ID da instalação"), "91");
    await user.click(screen.getByLabelText("Chave privada (PEM)"));
    await user.paste(KEY);
    await user.click(screen.getByRole("button", { name: "Conectar sistema" }));

    expect(sent()).toMatchObject({
      githubApp: { appId: 5550600, installationId: 91, privateKey: KEY },
    });
    expect(sent().token).toBeUndefined();
  });
});
