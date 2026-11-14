import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import ConsoleRoutes from "@/routes/console-routes";

// The integrations are listed and configured at the platform's scope only. An
// administrator of one area who follows a link there must read why, not a page
// of requests answering 403.

function signedInAs(platform: string[], can = ["tool:read", "scope:write"], mcpServers: object[] = []) {
  const me = {
    id: "usr_ana", display: "Ana", kind: "human",
    can, platform, grants: [],
    configure: can.includes("provider:write") ? [{ company: "acme", area: "cx" }] : [],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.endsWith("/api/v1/me") ? me : { items: [], mcpServers, providers: [] };
      return new Response(JSON.stringify(body), {
        status: 200, headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

afterEach(() => vi.unstubAllGlobals());

// The screens are lazy chunks. Alone a test finds them inside the default
// second; beside the rest of the suite the import alone can take longer.
const LAZY_SCREEN = 5_000;

function openAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <ConsoleRoutes />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("integrations for an administrator of one area", () => {
  it("say they are configured by the platform's administration", async () => {
    signedInAs([]);
    openAt("/integrations/connectors");

    expect(
      await screen.findByText(
        "As integrações são configuradas pela administração da plataforma",
        {},
        { timeout: LAZY_SCREEN },
      ),
    ).toBeInTheDocument();
  });

  it("open as usual for someone who holds tool:read at the platform", async () => {
    signedInAs(["tool:read"]);
    openAt("/integrations");

    expect(
      await screen.findByRole(
        "heading",
        { level: 1, name: "Integrações" },
        { timeout: LAZY_SCREEN },
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("As integrações são configuradas pela administração da plataforma"),
    ).not.toBeInTheDocument();
  });
});

// NT-019 changes nothing for the platform's administration: every section,
// and connecting starts at the catalogue.
describe("integrations for the platform's administration", () => {
  it("show every section and the way to connect a system", async () => {
    signedInAs(["tool:read", "provider:write"], ["tool:read", "provider:write"]);
    openAt("/integrations");

    expect(
      await screen.findByRole("button", { name: "Conectar sistema" }, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    for (const section of ["Conectados", "Disponíveis", "Connectors", "Provedores de modelo", "Canais"]) {
      expect(screen.getByRole("link", { name: section }), section).toBeInTheDocument();
    }
  });
});

// A reader of the platform's who configures nowhere reads every section and
// is offered nothing to create: the write would answer 403 (NT-019).
describe("integrations for the platform's auditor", () => {
  it("offer no new server in the connected list", async () => {
    signedInAs(["tool:read"], ["tool:read"]);
    openAt("/integrations");

    expect(
      await screen.findByText("Nenhum servidor configurado", {}, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Novo" })).not.toBeInTheDocument();
  });

  it("keep the catalogue readable, with no way to connect by address", async () => {
    signedInAs(["tool:read"], ["tool:read"]);
    openAt("/integrations/mcp");

    expect(
      await screen.findByLabelText("Buscar servidor ou ferramenta", {}, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Conectar por endereço" })).not.toBeInTheDocument();
  });
});

// NT-019: an area's administrator configures its own connector instances and
// its own MCP servers.
describe("integrations for an administrator who configures an area's connectors", () => {
  const areaAdmin = ["tool:read", "scope:write", "provider:write"];

  it("open the connected MCP servers at /integrations, where they can add one", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations");

    expect(
      await screen.findByText("Nenhum servidor configurado", {}, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Novo" })).toBeInTheDocument();
  });

  it("show them their own server with its area, and an inherited one without its actions", async () => {
    signedInAs([], areaAdmin, [
      { name: "crm", transport: "http", enabled: true, scopeKind: "area", company: "acme", area: "cx", editable: true },
      { name: "docs", transport: "http", enabled: true, scopeKind: "installation", editable: false },
    ]);
    openAt("/integrations");

    expect(await screen.findByText("acme/cx", {}, { timeout: LAZY_SCREEN })).toBeInTheDocument();
    expect(screen.getByText("Da plataforma")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Editar" })).toHaveLength(1);
  });

  it.each(["/integrations/providers", "/integrations/channels"])(
    "take them from %s, which is the platform's, to the connected servers",
    async (path) => {
      signedInAs([], areaAdmin);
      openAt(path);

      expect(
        await screen.findByText("Nenhum servidor configurado", {}, { timeout: LAZY_SCREEN }),
      ).toBeInTheDocument();
      expect(
        screen.queryByText("As integrações são configuradas pela administração da plataforma"),
      ).not.toBeInTheDocument();
    },
  );

  it("offer them no way to connect what is the platform's", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations/connectors");

    expect(
      await screen.findByRole("button", { name: "Nova instância" }, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Conectar sistema" })).not.toBeInTheDocument();
  });

  it("show them what they connect and their credentials, and none of the platform's sections", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations/connectors");

    expect(
      await screen.findByRole("link", { name: "Connectors" }, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    for (const section of ["Conectados", "Disponíveis", "Credenciais"]) {
      expect(screen.getByRole("link", { name: section }), section).toBeInTheDocument();
    }
    for (const section of ["Provedores de modelo", "Canais"]) {
      expect(screen.queryByRole("link", { name: section }), section).not.toBeInTheDocument();
    }
  });

  // Connecting a server starts at the catalogue for them too: the recipes
  // name nothing of this installation, and the form places the server only
  // where they may.
  it("open the catalogue, where they connect a server by its address too", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations/mcp");

    expect(
      await screen.findByRole("button", { name: "Conectar por endereço" }, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
  });

  it("open their personal credentials", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations/credentials");

    expect(
      await screen.findByRole("link", { name: "Credenciais" }, { timeout: LAZY_SCREEN }),
    ).toHaveAttribute("aria-current", "page");
  });

  // Channels are the platform's, and would answer 403. The listing of MCP
  // servers, the catalogue and their own credentials are theirs.
  it("ask for none of the platform's lists", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations");
    await screen.findByText("Nenhum servidor configurado", {}, { timeout: LAZY_SCREEN });

    const asked = vi.mocked(fetch).mock.calls.map(([input]) =>
      new URL(input instanceof Request ? input.url : String(input), "http://console").pathname,
    );
    expect(asked.some((path) => path.endsWith("/admin/integrations"))).toBe(true);
    expect(asked.filter((path) => path.endsWith("/admin/channels"))).toEqual([]);
    for (const own of ["/admin/integrations/recipes", "/integrations/mcp-credentials"]) {
      expect(asked.some((path) => path.endsWith(own)), own).toBe(true);
    }
  });
});

// Before the session answers, the gate says neither yes nor no: a guess shows
// an area's administrator that integrations are the platform's, a moment
// before it opens their connectors.
describe("integrations before the session answers", () => {
  it("wait, and say nothing about who may open them", async () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => {})));
    openAt("/integrations/connectors");

    expect(await screen.findByText("Carregando", {}, { timeout: LAZY_SCREEN })).toBeInTheDocument();
    expect(
      screen.queryByText("As integrações são configuradas pela administração da plataforma"),
    ).not.toBeInTheDocument();
  });
});
