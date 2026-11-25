import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ServerForm } from "@/features/integrations/server-form";
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

// The catalogue answers with no recipe for these servers, a custom server,
// unless a test says it has not answered yet or answers with one.
const catalogue = vi.hoisted(() => ({ data: { items: [] } as { items: unknown[] } | undefined }));
vi.mock("@/features/integrations/mcp/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/integrations/mcp/api")>();
  return { ...actual, useRecipes: () => ({ data: catalogue.data }) };
});

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function server(): MCPServer {
  return {
    name: "github",
    transport: "http",
    url: "https://api.githubcopilot.com/mcp/",
    enabled: true,
    hasSecret: true,
    hasOAuth: true,
  };
}

describe("the MCP server properties sheet", () => {
  beforeEach(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
    api.mutateAsync.mockReset();
    api.mutateAsync.mockResolvedValue(undefined);
    catalogue.data = { items: [] };
  });

  it("keeps the connection form full-height with a scrollable body", () => {
    render(<ServerForm server={server()} onClose={vi.fn()} />);

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveClass(
      "h-full",
      "overflow-hidden",
      "right-0",
    );
    expect(screen.getByTestId("server-form-scroll")).toHaveClass(
      "overflow-y-auto",
      "min-h-0",
      "flex-1",
    );
    expect(screen.getByRole("button", { name: "Salvar" })).toBeInTheDocument();
  });

  it("shows the stored per-worker rate limit", () => {
    render(
      <ServerForm
        server={{
          ...server(),
          rateLimit: { ratePerSecond: 0.5, burst: 3 },
          cache: { ttlSeconds: 45, maxEntries: 80 },
        }}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Chamadas por segundo")).toHaveValue("0.5");
    expect(screen.getByLabelText("Rajada")).toHaveValue("3");
    expect(screen.getByLabelText("TTL da cache em segundos")).toHaveValue("45");
    expect(screen.getByLabelText("Entradas da cache")).toHaveValue("80");
  });

  it("saves stdio egress as structured destinations", async () => {
    const user = userEvent.setup();

    render(
      <ServerForm
        server={{
          name: "crm",
          transport: "stdio",
          command: "/bin/crm-mcp",
          args: [],
          enabled: true,
          acceptsLocalExecution: true,
        }}
        onClose={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("combobox", { name: "Egresso do processo local" }),
    );
    await user.click(
      await screen.findByRole("option", { name: "Exigir proxy de egresso" }),
    );
    await user.type(
      screen.getByLabelText("Destinos permitidos"),
      "CRM.Internal:443\n*.sales.internal:8443",
    );
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        stdioEgress: {
          mode: "proxied",
          allowedDestinations: [
            { host: "crm.internal", port: 443 },
            { host: "*.sales.internal", port: 8443 },
          ],
        },
      }),
    );
  });
  describe("the credential's shape", () => {
    it("opens on the one stored, with only its fields", () => {
      render(<ServerForm server={server()} onClose={vi.fn()} />);
      expect(screen.getByRole("radio", { name: /^OAuth/ })).toBeChecked();
      expect(screen.queryByLabelText("Token bearer")).not.toBeInTheDocument();
      expect(screen.getByLabelText(/access token oauth/i)).toBeInTheDocument();
    });

    it("keeps the OAuth grant's rarely needed fields under Advanced", async () => {
      const user = userEvent.setup();
      render(<ServerForm server={server()} onClose={vi.fn()} />);
      expect(screen.queryByLabelText(/expira em/i)).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /avançado/i }));
      expect(screen.getByLabelText(/expira em/i)).toBeInTheDocument();
    });

    // The client typed in alone belongs to the browser sign-in; saved here it
    // replaced the credential that worked.
    it("refuses a manual grant with no token, and sends nothing", async () => {
      const user = userEvent.setup();
      render(<ServerForm server={server()} onClose={vi.fn()} />);
      await user.type(screen.getByLabelText("Client ID OAuth"), "agents");
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      expect(await screen.findByText(/não tem o que enviar/)).toBeInTheDocument();
      expect(api.mutateAsync).not.toHaveBeenCalled();
    });

    it("starts a shape empty after switching away from it and back", async () => {
      const user = userEvent.setup();
      render(<ServerForm server={server()} onClose={vi.fn()} />);
      await user.type(screen.getByLabelText(/access token oauth/i), "access");
      await user.click(screen.getByRole("radio", { name: /^Token/ }));
      await user.click(screen.getByRole("radio", { name: /^OAuth/ }));
      expect(screen.getByLabelText(/access token oauth/i)).toHaveValue("");
    });
  });

  // A recipe that arrives after the sheet can change the shape shown with
  // nobody touching it: what was typed under the old one goes with it, or an
  // invalid value nobody can see any more would refuse every save.
  it("clears the shape a late recipe takes away, so the one it offers saves", async () => {
    const user = userEvent.setup();
    catalogue.data = undefined;
    const docs: MCPServer = { name: "docs", transport: "http", url: "https://docs.example/mcp", enabled: true };
    const { rerender } = render(<ServerForm server={docs} onClose={vi.fn()} />);
    await user.click(screen.getByRole("radio", { name: /^OAuth/ }));
    await user.click(screen.getByRole("button", { name: /avançado/i }));
    await user.type(screen.getByLabelText(/expira em/i), "soon");

    catalogue.data = { items: [{ server: "docs", title: "Docs", category: "knowledge",
      authModes: [{ type: "headers", principal: "service", headers: ["Api-Key"] }] }] };
    rerender(<ServerForm server={docs} onClose={vi.fn()} />);
    await user.type(await screen.findByLabelText("Api-Key"), "key");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({ headers: { "Api-Key": "key" } }));
  });

  describe("a custom header for a server with no recipe", () => {
    const generic = (): MCPServer => ({ name: "crm", transport: "http", url: "https://crm.example/mcp", enabled: true });

    async function fill(name: string, value: string) {
      const user = userEvent.setup();
      render(<ServerForm server={generic()} onClose={vi.fn()} />);
      await user.click(screen.getByRole("radio", { name: /^Cabeçalho próprio/ }));
      if (name) await user.type(screen.getByLabelText("Nome do header"), name);
      if (value) await user.type(screen.getByLabelText("Valor do header"), value);
      return user;
    }

    it("sends the header as the credential, and no bearer", async () => {
      const user = await fill("apikey", "k-123");
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      expect(api.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({ headers: { apikey: "k-123" } }));
      expect(api.mutateAsync.mock.calls[0]?.[0]).not.toHaveProperty("token", expect.anything());
    });

    // One shape at a time: a token typed before switching is never sent.
    it("sends the header alone after a token was typed under another shape", async () => {
      const user = userEvent.setup();
      render(<ServerForm server={generic()} onClose={vi.fn()} />);
      await user.type(screen.getByLabelText("Token bearer"), "t-1");
      await user.click(screen.getByRole("radio", { name: /^Cabeçalho próprio/ }));
      await user.type(screen.getByLabelText("Nome do header"), "apikey");
      await user.type(screen.getByLabelText("Valor do header"), "k-123");
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      expect(api.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({ headers: { apikey: "k-123" } }));
      expect(api.mutateAsync.mock.calls[0]?.[0]).not.toHaveProperty("token", expect.anything());
    });

    it.each([
      ["a name without a value", "apikey", "", "Preencha o nome e o valor do header."],
      ["a value without a name", "", "k-123", "Preencha o nome e o valor do header."],
      ["a name HTTP cannot send", "api key", "k-123", "Use só letras, números e - _ . no nome do header."],
    ])("refuses %s", async (_case, name, value, message) => {
      const user = await fill(name, value);
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      expect(await screen.findByText(message)).toBeInTheDocument();
      expect(api.mutateAsync).not.toHaveBeenCalled();
    });
  });
});
