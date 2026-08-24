import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, beforeEach, vi } from "vitest";
import { ConnectionPanel } from "@/features/integrations/mcp/connection-panel";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import ptBR from "@/i18n/pt-BR.json";
import enUS from "@/i18n/en-US.json";

const api = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  probeAsync: vi.fn(),
}));

const PT_EGRESS_METADATA =
  "Endereços de metadata de nuvem e link-local são recusados. Proxies de ambiente são recusados para que a validação de endereço use DNS local. Destinos na rede privada e na internet continuam permitidos.";
const PT_EGRESS_PROXY =
  "Este processo local recebe HTTP_PROXY e HTTPS_PROXY para um proxy local do worker que recusa destinos fora da allow-list. Isso ainda não impede sockets diretos sem uma política de rede no deploy.";
const PT_EGRESS_PROXY_WITH_NETWORK_POLICY =
  "Este processo local usa o proxy local do worker, que recusa destinos fora da allow-list, e o operador declarou que os pods de worker estão cobertos por NetworkPolicy aplicada pelo cluster. O FuseOne não verifica o CNI; esta afirmação depende dessa declaração operacional.";
const PT_EGRESS_LOCAL =
  "O FuseOne não restringe destinos de saída deste processo local. Ele roda com o acesso de rede do worker.";
const PT_EGRESS_UNKNOWN =
  "Este servidor observado pelo worker não tem transporte configurado no console, então o FuseOne não consegue descrever sua política de egresso.";
const EN_EGRESS_METADATA =
  "Cloud metadata and link-local addresses are refused. Environment proxies are refused so address validation uses local DNS. Private network and internet destinations are otherwise allowed.";
const EN_EGRESS_PROXY =
  "This local process receives HTTP_PROXY and HTTPS_PROXY for a worker-local proxy that refuses destinations outside its allow-list. This still does not prevent direct sockets without a deployment network policy.";
const EN_EGRESS_PROXY_WITH_NETWORK_POLICY =
  "This local process uses the worker-local proxy that refuses destinations outside its allow-list, and the operator declared that worker pods are covered by cluster-enforced NetworkPolicy. FuseOne does not verify the CNI; this statement depends on that operational declaration.";
const EN_EGRESS_LOCAL =
  "FuseOne does not constrain outbound destinations for this local process. It runs with the worker's network access.";
const EN_EGRESS_UNKNOWN =
  "This worker-observed server has no configured transport in the console, so FuseOne cannot describe its egress policy.";

vi.mock("@/features/integrations/api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/integrations/api")>();
  return {
    ...actual,
    usePutMCPServer: () => ({ mutateAsync: api.mutateAsync, isPending: false }),
    useProbeMCPServer: () => ({ mutateAsync: api.probeAsync, isPending: false }),
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
    requiresPersonalCredential: false,
    authModes,
    transport: "http",
    url: "https://mcp.example.com/google",
  };
}

describe("the MCP connection panel", () => {
  beforeEach(() => {
    api.mutateAsync.mockReset();
    api.mutateAsync.mockResolvedValue(undefined);
    api.probeAsync.mockReset();
    api.probeAsync.mockResolvedValue(undefined);
  });

  it("asks the worker to try the connection without rewriting credentials", async () => {
    render(<ConnectionPanel server={remote({ name: "stripe" })} />);

    await userEvent.click(screen.getByRole("button", { name: "Tentar agora" }));

    expect(api.probeAsync).toHaveBeenCalledWith("stripe");
    expect(api.mutateAsync).not.toHaveBeenCalled();
  });

  it("does not offer a worker check for a disabled connection", () => {
    render(<ConnectionPanel server={remote({ enabled: false })} />);

    expect(screen.getByRole("button", { name: "Tentar agora" })).toBeDisabled();
  });

  it("explains when discovery can work but calls need this user's credential", () => {
    render(
      <ConnectionPanel
        server={remote({
          hasSecret: true,
          callAuth: {
            policy: "personal_required",
            callerHasPersonalCredential: false,
          },
        })}
      />,
    );

    expect(screen.getByText("Uso em chamadas")).toBeInTheDocument();
    expect(screen.getByText(/chamadas de ferramenta em seu nome vão parar/i))
      .toBeInTheDocument();
  });

  it("does not pretend a custom HTTP server's auth shape is verified", () => {
    render(
      <ConnectionPanel
        server={remote({
          callAuth: {
            policy: "unknown",
            callerHasPersonalCredential: false,
          },
        })}
      />,
    );

    expect(screen.getByText(/Nenhuma receita do catálogo verifica esta forma de autenticação/i))
      .toBeInTheDocument();
  });

  it("says a local process cannot use per-user credentials", () => {
    render(
      <ConnectionPanel
        server={{
          name: "local-wiki",
          transport: "stdio",
          command: "wiki-mcp",
          args: [],
          enabled: true,
          callAuth: {
            policy: "local_process",
            callerHasPersonalCredential: false,
          },
        }}
      />,
    );

    expect(screen.getByText(/não há credencial por usuário nesse transporte/i))
      .toBeInTheDocument();
  });

  it("pins the egress statements in both locales", () => {
    expect(ptBR.mcp.egressMetadataRefused).toBe(PT_EGRESS_METADATA);
    expect(ptBR.mcp.egressProxyRequested).toBe(PT_EGRESS_PROXY);
    expect(ptBR.mcp.egressProxyWithNetworkPolicy).toBe(
      PT_EGRESS_PROXY_WITH_NETWORK_POLICY,
    );
    expect(ptBR.mcp.egressLocalUnconstrained).toBe(PT_EGRESS_LOCAL);
    expect(ptBR.mcp.egressUnknown).toBe(PT_EGRESS_UNKNOWN);
    expect(enUS.mcp.egressMetadataRefused).toBe(EN_EGRESS_METADATA);
    expect(enUS.mcp.egressProxyRequested).toBe(EN_EGRESS_PROXY);
    expect(enUS.mcp.egressProxyWithNetworkPolicy).toBe(
      EN_EGRESS_PROXY_WITH_NETWORK_POLICY,
    );
    expect(enUS.mcp.egressLocalUnconstrained).toBe(EN_EGRESS_LOCAL);
    expect(enUS.mcp.egressUnknown).toBe(EN_EGRESS_UNKNOWN);
  });

  it("says HTTP refuses metadata but does not claim egress containment", () => {
    render(
      <ConnectionPanel
        server={remote({
          egress: { policy: "metadata_refused" },
        })}
      />,
    );

    expect(screen.getByText("Alcance de rede")).toBeInTheDocument();
    expect(screen.getByText(PT_EGRESS_METADATA)).toBeInTheDocument();
  });

  it("says stdio has the worker network and no platform egress constraint", () => {
    render(
      <ConnectionPanel
        server={{
          name: "local-wiki",
          transport: "stdio",
          command: "wiki-mcp",
          args: [],
          enabled: true,
          egress: { policy: "unconstrained_local_process" },
        }}
      />,
    );

    expect(screen.getByText(PT_EGRESS_LOCAL)).toBeInTheDocument();
  });

  it("says proxied stdio is a proxy request rather than direct containment", () => {
    render(
      <ConnectionPanel
        server={{
          name: "local-wiki",
          transport: "stdio",
          command: "wiki-mcp",
          args: [],
          enabled: true,
          egress: { policy: "proxy_requested" },
        }}
      />,
    );

    expect(screen.getByText(PT_EGRESS_PROXY)).toBeInTheDocument();
  });

  it("says proxied stdio has deployment containment only after the operator declaration", () => {
    render(
      <ConnectionPanel
        server={{
          name: "local-wiki",
          transport: "stdio",
          command: "wiki-mcp",
          args: [],
          enabled: true,
          egress: { policy: "proxy_with_network_policy" },
        }}
      />,
    );

    expect(screen.getByText(PT_EGRESS_PROXY_WITH_NETWORK_POLICY))
      .toBeInTheDocument();
  });

  it("says observed unmanaged servers have unknown egress policy", () => {
    render(
      <ConnectionPanel
        server={remote({
          egress: { policy: "unknown" },
        })}
      />,
    );

    expect(screen.getByText(PT_EGRESS_UNKNOWN)).toBeInTheDocument();
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

  it("stores a named custom header as headers rather than a bearer token", async () => {
    render(
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

    await userEvent.type(screen.getByLabelText(/new relic api key/i), "nr_secret");
    await userEvent.click(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        token: undefined,
        headers: { "Api-Key": "nr_secret" },
      }),
    );
  });

  it("stores Basic auth as the Authorization header value", async () => {
    render(
      <ConnectionPanel
        server={remote({ name: "atlassian" })}
        recipe={recipe([
          {
            type: "basic",
            principal: "user",
            label: "Personal API token",
            header: "Authorization",
            prefix: "Basic",
          },
        ])}
      />,
    );

    await userEvent.type(screen.getByLabelText(/personal api token/i), "encoded");
    await userEvent.click(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        token: undefined,
        headers: { Authorization: "Basic encoded" },
      }),
    );
  });

  it("stores a named multi-header credential as exact headers", async () => {
    const { container } = render(
      <ConnectionPanel
        server={remote({ name: "datadog" })}
        recipe={recipe([
          {
            type: "headers",
            principal: "service",
            label: "API and application key headers",
            headers: ["DD_API_KEY", "DD_APPLICATION_KEY"],
          },
        ])}
      />,
    );

    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("DD_API_KEY"), "api_secret");
    await userEvent.type(
      screen.getByLabelText("DD_APPLICATION_KEY"),
      "app_secret",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        token: undefined,
        headers: {
          DD_API_KEY: "api_secret",
          DD_APPLICATION_KEY: "app_secret",
        },
      }),
    );
  });

  it("saves multi-header auth when a bearer alternative is also documented", async () => {
    render(
      <ConnectionPanel
        server={remote({ name: "datadog" })}
        recipe={recipe([
          {
            type: "bearer",
            principal: "service",
            label: "Service access token",
          },
          {
            type: "headers",
            principal: "service",
            label: "API and application key headers",
            headers: ["DD_API_KEY", "DD_APPLICATION_KEY"],
          },
        ])}
      />,
    );

    await userEvent.type(screen.getByLabelText("DD_API_KEY"), "api_secret");
    await userEvent.type(
      screen.getByLabelText("DD_APPLICATION_KEY"),
      "app_secret",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        token: undefined,
        headers: {
          DD_API_KEY: "api_secret",
          DD_APPLICATION_KEY: "app_secret",
        },
      }),
    );
  });

  it("does not choose between bearer and multi-header auth on behalf of the operator", async () => {
    render(
      <ConnectionPanel
        server={remote({ name: "datadog" })}
        recipe={recipe([
          {
            type: "bearer",
            principal: "service",
            label: "Service access token",
          },
          {
            type: "headers",
            principal: "service",
            label: "API and application key headers",
            headers: ["DD_API_KEY", "DD_APPLICATION_KEY"],
          },
        ])}
      />,
    );

    await userEvent.type(screen.getByLabelText(/service access token/i), "sat");
    await userEvent.type(screen.getByLabelText("DD_API_KEY"), "api_secret");

    expect(
      screen.getByText(/preencha só uma forma de credencial/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    ).toBeDisabled();
  });

  it("does not pretend unshaped header auth is editable", () => {
    const { container } = render(
      <ConnectionPanel
        server={remote({ name: "datadog" })}
        recipe={recipe([
          {
            type: "headers",
            principal: "service",
            label: "API and application key headers",
          },
        ])}
      />,
    );

    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(
      screen.getByText(/espera API and application key headers/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    ).toBeDisabled();
  });

  it("stores a local DSN in the env variable the recipe names", async () => {
    render(
      <ConnectionPanel
        server={remote({
          name: "postgres",
          transport: "stdio",
          command: "postgres-mcp",
          url: undefined,
        })}
        recipe={{
          ...recipe([
            {
              type: "dsn",
              principal: "service",
              label: "PostgreSQL connection string",
              env: "DATABASE_URL",
            },
          ]),
          server: "postgres",
          title: "PostgreSQL",
          transport: "stdio",
          command: "postgres-mcp",
          url: undefined,
        }}
      />,
    );

    await userEvent.type(
      screen.getByLabelText(/postgresql connection string/i),
      "postgres://readonly@example/db",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Salvar a credencial" }),
    );

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        env: { DATABASE_URL: "postgres://readonly@example/db" },
      }),
    );
  });
});
