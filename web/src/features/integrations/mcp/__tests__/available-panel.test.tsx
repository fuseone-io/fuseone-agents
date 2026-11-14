import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AvailableServersPanel } from "@/features/integrations/mcp/available-servers-panel";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import type { MCPServer } from "@/features/integrations/api";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";

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

const stripe: ServerRecipe = {
  server: "stripe",
  title: "Stripe",
  category: "finance",
  publisher: "Stripe",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "published",
  configRequirements: ["credential"],
  requiresPersonalCredential: true,
  authModes: [{ type: "oauth2", principal: "user", label: "Stripe OAuth" }],
  transport: "http",
  url: "https://mcp.stripe.com",
  docs: "https://docs.stripe.com/",
  note: "Payments and billing.",
};

const postgres: ServerRecipe = {
  server: "postgres",
  title: "PostgreSQL",
  category: "data",
  publisher: "Model Context Protocol reference servers",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "archived",
  configRequirements: ["credential"],
  requiresPersonalCredential: false,
  authModes: [
    {
      type: "dsn",
      principal: "service",
      label: "PostgreSQL DSN",
      env: "DATABASE_URL",
    },
  ],
  note: "Read-only database access.",
};

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
  note: "Observability.",
};

const datadogWithBearer: ServerRecipe = {
  ...datadog,
  authModes: [
    {
      type: "bearer",
      principal: "service",
      label: "Service access token",
    },
    ...(datadog.authModes ?? []),
  ],
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
  authModes: [{ type: "bearer", principal: "user", label: "Outline API key" }],
  transport: "http",
  protocolMode: "legacy",
  url: "https://example.getoutline.com/mcp",
  note: "Workspace wiki.",
};

function open(recipes: ServerRecipe[] = [stripe], canCreate?: boolean, servers: MCPServer[] = []) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AvailableServersPanel
          servers={servers}
          recipes={recipes}
          isLoading={false}
          error={null}
          onRetry={vi.fn()}
          canCreate={canCreate}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/*
The catalogue card opens the configuration panel beside the list.

Routing to `/integrations/mcp/new?recipe=stripe` lost the user's place in the
catalogue and put the form under another grid of recipes. The handoff's shape
is more specific: the selected server stays in view and the right panel holds
the connection act for exactly that server.
*/
describe("available MCP servers", () => {
  beforeEach(() => {
    api.mutateAsync.mockReset();
    api.mutateAsync.mockResolvedValue(undefined);
  });

  it("opens the selected recipe in the side configuration panel", async () => {
    const { container } = open();

    expect(
      container.querySelector('[data-mcp-icon="stripe"] svg'),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Conectados/ }).querySelector("svg"),
    ).not.toBeNull();
    expect(screen.getByText("Catálogo MCP").querySelector("svg")).not.toBeNull();
    expect(screen.getByText("Sincronizações").querySelector("svg")).not.toBeNull();
    for (const name of ["Todos", "Publicados", "Referência", "Arquivados"]) {
      expect(screen.getByRole("button", { name }).querySelector("svg")).not.toBeNull();
    }

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar Stripe" }),
    );

    expect(screen.getByRole("heading", { name: "Stripe" })).toBeInTheDocument();
    expect(screen.getAllByText("Stripe OAuth").length).toBeGreaterThan(0);
    expect(screen.getByDisplayValue("stripe")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://mcp.stripe.com")).toBeInTheDocument();
    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/access token oauth/i)).toBeInTheDocument();
  });

  it("does not let an archived reference recipe look current", async () => {
    const { container } = open([postgres]);

    expect(screen.getByText("arquivado")).toBeInTheDocument();
    expect(screen.getByText("credencial")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar PostgreSQL" }),
    );

    expect(screen.getAllByText("Read-only database access.")).toHaveLength(2);
    expect(
      screen.getByText(/marcou este servidor como arquivado/),
    ).toBeInTheDocument();
    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/postgresql dsn/i)).toBeInTheDocument();
  });

  it("opens a multi-header recipe as separate header fields", async () => {
    const { container } = open([datadog]);

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar Datadog" }),
    );

    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText("DD_API_KEY")).toBeInTheDocument();
    expect(screen.getByLabelText("DD_APPLICATION_KEY")).toBeInTheDocument();
  });

  it("connects a multi-header recipe by storing exact headers", async () => {
    open([datadog]);

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar Datadog" }),
    );
    await userEvent.type(screen.getByLabelText("DD_API_KEY"), "api_secret");
    await userEvent.type(
      screen.getByLabelText("DD_APPLICATION_KEY"),
      "app_secret",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Conectar sistema" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "datadog",
        headers: {
          DD_API_KEY: "api_secret",
          DD_APPLICATION_KEY: "app_secret",
        },
      }),
    );
    expect(api.mutateAsync.mock.calls[0]?.[0].token).toBeUndefined();
  });

  it("connects a legacy HTTP recipe with its protocol mode", async () => {
    open([outline]);

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar Outline" }),
    );
    await userEvent.type(screen.getByLabelText(/outline api key/i), "pat");
    await userEvent.click(
      screen.getByRole("button", { name: "Conectar sistema" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "outline",
        transport: "http",
        protocolMode: "legacy",
        url: "https://example.getoutline.com/mcp",
        token: "pat",
      }),
    );
  });

  it("refuses the initial-connect credential conflict instead of choosing one", async () => {
    open([datadogWithBearer]);

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar Datadog" }),
    );
    await userEvent.type(screen.getByLabelText(/service access token/i), "sat");
    await userEvent.type(screen.getByLabelText("DD_API_KEY"), "api_secret");
    await userEvent.click(
      screen.getByRole("button", { name: "Conectar sistema" }),
    );

    expect(api.mutateAsync).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/preencha só uma forma de credencial/i),
    ).toBeInTheDocument();
    expect(api.mutateAsync).not.toHaveBeenCalled();
  });
});

// A reader of the platform's who configures nowhere reads the catalogue and
// is offered no way to connect: the write would answer 403 (NT-019).
describe("the catalogue for someone who may not create a server", () => {
  it("opens a recipe to read, with no form and no connect", async () => {
    open([stripe], false);
    expect(screen.queryByRole("button", { name: "Conectar por endereço" })).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Ver Stripe" }));

    expect(screen.getAllByText("Payments and billing.")).toHaveLength(2);
    expect(screen.getByText(/Conectar um servidor pede provider:write/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Conectar sistema" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Nome")).not.toBeInTheDocument();
  });

  // A connected server is read too: "Configurar" would open a page that
  // answers 403 to whoever may not change it.
  it("opens a connected server to read, with no configure", async () => {
    const connected = { name: "stripe", transport: "http", url: "https://mcp.stripe.com", enabled: true } as MCPServer;
    open([stripe], false, [connected]);
    expect(screen.queryByText("Configurar")).not.toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Ver Stripe" }));

    expect(screen.queryByRole("button", { name: "Configurar" })).not.toBeInTheDocument();
    expect(screen.getByText(/Conectar um servidor pede provider:write/)).toBeInTheDocument();
  });
});

// Whoever may create somewhere still reads a server they may not change: one
// of another scope (editable false) or one passed to the process (managed
// false), whose page has no row to change (NT-019).
describe("the catalogue for someone who may create, on a server they may not change", () => {
  for (const [why, flags] of [["of another scope", { editable: false }], ["not managed here", { managed: false }]] as const) {
    it(`reads a server ${why}, with no configure`, async () => {
      const server = { name: "stripe", transport: "http", url: "https://mcp.stripe.com", enabled: true, ...flags } as MCPServer;
      open([stripe], true, [server]);
      expect(screen.queryByText("Configurar")).not.toBeInTheDocument();
      await userEvent.setup().click(screen.getByRole("button", { name: "Ver Stripe" }));

      expect(screen.queryByRole("button", { name: "Configurar" })).not.toBeInTheDocument();
      expect(screen.getByText(/não pode alterá-lo aqui/)).toBeInTheDocument();
    });
  }
});

// An area connects remote servers only (NT-019): a local process runs on the
// platform's worker, and the server refuses one stored outside the
// installation. A recipe reached only by a local command is read, not
// offered to connect.
describe("the catalogue for someone who connects only an area's servers", () => {
  function openInArea(recipes: ServerRecipe[]) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <ScopeChoiceContext.Provider value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }] }}>
            <AvailableServersPanel servers={[]} recipes={recipes} isLoading={false} error={null} onRetry={vi.fn()} />
          </ScopeChoiceContext.Provider>
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it("reads a local-only recipe, saying local servers are the platform's", async () => {
    openInArea([postgres]);
    await userEvent.setup().click(screen.getByRole("button", { name: "Ver PostgreSQL" }));

    expect(screen.getByText(/Servidores locais são da plataforma/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Conectar sistema" })).not.toBeInTheDocument();
  });

  it("still connects a remote recipe", async () => {
    openInArea([stripe]);
    await userEvent.setup().click(screen.getByRole("button", { name: "Conectar Stripe" }));

    expect(screen.getByRole("button", { name: "Conectar sistema" })).toBeInTheDocument();
    expect(screen.queryByText(/Servidores locais são da plataforma/)).not.toBeInTheDocument();
  });
});
