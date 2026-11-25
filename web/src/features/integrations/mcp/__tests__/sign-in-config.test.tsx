import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ConnectionPanel } from "@/features/integrations/mcp/connection-panel";
import type { MCPServer } from "@/features/integrations/api";

/*
A remote server's browser sign-in, configured on its page: the client people
sign in through, its secret kept apart and never shown, and the address the
provider must send them back to. Saving sends the configuration whole, with a
secret left blank kept; removing it is its own gesture.
*/

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

function remote(overrides: Partial<MCPServer> = {}): MCPServer {
  return { name: "dpg", transport: "http", url: "https://mcp.example.com/mcp", enabled: true, ...overrides };
}

function section() {
  return screen.getByRole("region", { name: "Login pelo navegador" });
}

function sent() {
  return api.mutateAsync.mock.calls.at(-1)?.[0];
}

describe("the browser sign-in on a server's page", () => {
  beforeEach(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.scrollIntoView ??= () => {};
    api.mutateAsync.mockReset();
    api.mutateAsync.mockResolvedValue(undefined);
  });

  it("says where the provider must send people back to", () => {
    render(<ConnectionPanel server={remote()} />);
    expect(within(section()).getByText(`${window.location.origin}/auth/mcp/callback`)).toBeInTheDocument();
  });

  it("saves a new sign-in with its client, secret and scopes", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={remote()} />);
    await user.type(within(section()).getByLabelText("Client ID"), "data-platform");
    await user.type(within(section()).getByLabelText("Client secret"), "s3cret");
    await user.type(within(section()).getByLabelText("Escopos"), "openid offline_access");
    await user.click(within(section()).getByRole("button", { name: "Salvar o login pelo navegador" }));

    expect(sent().oauthLogin).toEqual({
      clientID: "data-platform", clientSecret: "s3cret", scopes: ["openid", "offline_access"],
    });
    expect(sent().token).toBeUndefined();
  });

  // The configuration is written whole: opened on what is stored, an edit of
  // one field sends the others back, and a secret left blank is kept.
  it("opens on the stored sign-in and keeps its secret when left blank", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={remote({
      oauthLogin: { clientID: "data-platform", scopes: ["openid"], tokenURL: "https://relay.example.com/token",
        tokenAuthMethod: "client_secret_basic", hasClientSecret: true },
    })} />);
    expect(within(section()).getByText(/client secret guardado/i)).toBeInTheDocument();
    const scopes = within(section()).getByLabelText("Escopos");
    await user.clear(scopes);
    await user.type(scopes, "openid profile");
    await user.click(within(section()).getByRole("button", { name: "Salvar o login pelo navegador" }));

    expect(sent().oauthLogin).toEqual({
      clientID: "data-platform", scopes: ["openid", "profile"], tokenURL: "https://relay.example.com/token",
      tokenAuthMethod: "client_secret_basic",
    });
  });

  it("removes the sign-in with an empty object", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={remote({
      oauthLogin: { clientID: "data-platform", tokenAuthMethod: "none", hasClientSecret: false },
    })} />);
    await user.click(within(section()).getByRole("button", { name: "Remover o login pelo navegador" }));
    expect(sent().oauthLogin).toEqual({});
  });

  // A method that sends a secret with none left would be refused: removing
  // the secret sends the rest back with the method left to its default.
  it("removes the client secret alone, keeping the rest", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={remote({
      oauthLogin: { clientID: "data-platform", scopes: ["openid"], tokenAuthMethod: "client_secret_basic", hasClientSecret: true },
    })} />);
    await user.click(within(section()).getByRole("button", { name: "Remover o client secret" }));
    expect(sent().oauthLogin).toEqual({ clientID: "data-platform", scopes: ["openid"], clientSecret: "" });
  });

  it("refuses to save a sign-in with no client", async () => {
    const user = userEvent.setup();
    render(<ConnectionPanel server={remote()} />);
    await user.click(within(section()).getByRole("button", { name: "Salvar o login pelo navegador" }));
    expect(await within(section()).findByText("Informe o client ID.")).toBeInTheDocument();
    expect(api.mutateAsync).not.toHaveBeenCalled();
  });

  it("is not offered for a local process, nor for a server acting as a GitHub App", () => {
    const { unmount } = render(<ConnectionPanel server={{ name: "fs", transport: "stdio", command: "fs", enabled: true }} />);
    expect(screen.queryByRole("region", { name: "Login pelo navegador" })).not.toBeInTheDocument();
    unmount();
    render(<ConnectionPanel server={remote({ githubApp: { appId: 1, installationId: 2, hasPrivateKey: true } })} />);
    expect(screen.queryByRole("region", { name: "Login pelo navegador" })).not.toBeInTheDocument();
  });
});
