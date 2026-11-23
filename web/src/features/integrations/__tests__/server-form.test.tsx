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

// The catalogue answers with no recipe for these servers: a custom server.
vi.mock("@/features/integrations/mcp/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/integrations/mcp/api")>();
  return { ...actual, useRecipes: () => ({ data: { items: [] } }) };
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
  describe("a custom header for a server with no recipe", () => {
    const generic = (): MCPServer => ({ name: "crm", transport: "http", url: "https://crm.example/mcp", enabled: true });

    async function fill(name: string, value: string) {
      const user = userEvent.setup();
      render(<ServerForm server={generic()} onClose={vi.fn()} />);
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

    it("refuses a header beside a bearer token", async () => {
      const user = await fill("apikey", "k-123");
      await user.type(screen.getByLabelText("Token bearer"), "t-1");
      await user.click(screen.getByRole("button", { name: "Salvar" }));
      expect(await screen.findByText("Preencha só uma forma de credencial para este servidor.")).toBeInTheDocument();
      expect(api.mutateAsync).not.toHaveBeenCalled();
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
