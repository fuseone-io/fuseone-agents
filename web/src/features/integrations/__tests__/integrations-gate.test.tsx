import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import ConsoleRoutes from "@/routes/console-routes";

// The integrations are listed and configured at the platform's scope only. An
// administrator of one area who follows a link there must read why, not a page
// of requests answering 403.

function signedInAs(platform: string[], can = ["tool:read", "scope:write"]) {
  const me = {
    id: "usr_ana", display: "Ana", kind: "human",
    can, platform, grants: [],
    configure: can.includes("provider:write") ? [{ company: "acme", area: "cx" }] : [],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.endsWith("/api/v1/me") ? me : { items: [], mcpServers: [], providers: [] };
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

  it.each(["/integrations/providers", "/integrations/channels", "/integrations/mcp"])(
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

  it("show them the connectors and the connected servers, and none of the platform's sections", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations/connectors");

    expect(
      await screen.findByRole("link", { name: "Connectors" }, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Conectados" })).toBeInTheDocument();
    for (const section of ["Provedores de modelo", "Disponíveis", "Canais"]) {
      expect(screen.queryByRole("link", { name: section }), section).not.toBeInTheDocument();
    }
  });

  // Each of these is the platform's, and would answer 403. The listing of MCP
  // servers is theirs too now: it holds their own and what they inherit.
  it("ask for none of the platform's lists", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations");
    await screen.findByText("Nenhum servidor configurado", {}, { timeout: LAZY_SCREEN });

    const asked = vi.mocked(fetch).mock.calls.map(([input]) =>
      new URL(input instanceof Request ? input.url : String(input), "http://console").pathname,
    );
    expect(asked.some((path) => path.endsWith("/admin/integrations"))).toBe(true);
    const platformLists = ["/admin/channels", "/admin/integrations/recipes", "/integrations/mcp-credentials"];
    for (const list of platformLists) {
      expect(asked.filter((path) => path.endsWith(list)), list).toEqual([]);
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
