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

// NT-019: an area's administrator configures its own connector instances.
describe("integrations for an administrator who configures an area's connectors", () => {
  const areaAdmin = ["tool:read", "scope:write", "provider:write"];

  it.each(["/integrations", "/integrations/providers"])(
    "take them from %s to the connectors, where they can add one",
    async (path) => {
      signedInAs([], areaAdmin);
      openAt(path);

      expect(
        await screen.findByRole("button", { name: "Nova instância" }, { timeout: LAZY_SCREEN }),
      ).toBeInTheDocument();
      expect(
        screen.queryByText("As integrações são configuradas pela administração da plataforma"),
      ).not.toBeInTheDocument();
    },
  );

  it("show them the connectors and none of the platform's sections", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations/connectors");

    expect(
      await screen.findByRole("link", { name: "Connectors" }, { timeout: LAZY_SCREEN }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Provedores de modelo" })).not.toBeInTheDocument();
  });

  // Each of these is the platform's, and would answer 403.
  it("ask for none of the platform's lists", async () => {
    signedInAs([], areaAdmin);
    openAt("/integrations/connectors");
    await screen.findByRole("button", { name: "Nova instância" }, { timeout: LAZY_SCREEN });

    const asked = vi.mocked(fetch).mock.calls.map(([input]) =>
      new URL(input instanceof Request ? input.url : String(input), "http://console").pathname,
    );
    expect(asked.some((path) => path.endsWith("/admin/integrations/connectors/instances"))).toBe(true);
    const platformLists = [
      "/admin/integrations", "/admin/channels", "/admin/integrations/recipes", "/integrations/mcp-credentials",
    ];
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
