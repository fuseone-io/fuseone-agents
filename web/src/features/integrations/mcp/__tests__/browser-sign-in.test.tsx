import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { UserCredentialsPanel } from "@/features/integrations/mcp/user-credentials-panel";
import type { CredentialServer } from "@/features/integrations/mcp/credential-server";

/*
A person connecting their own account to a server from Credentials: the
button begins the sign-in and sends the browser to the provider; the browser
comes back to Credentials with the outcome, which is said once, on the server
it was for, and taken out of the address so a reload does not say it again.
*/

const api = vi.hoisted(() => ({ start: vi.fn(), go: vi.fn(), pending: false }));

vi.mock("@/features/integrations/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/integrations/api")>();
  return {
    ...actual,
    usePutMCPUserCredential: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useDeleteMCPUserCredential: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useStartMCPSignIn: () => ({ mutateAsync: api.start, isPending: api.pending }),
  };
});
vi.mock("@/features/integrations/mcp/browser-navigation", () => ({ sendBrowserTo: api.go }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const signIn: CredentialServer = { name: "dpg", transport: "http", signIn: true, requiresPersonal: true };
const plain: CredentialServer = { name: "alpha", transport: "http" };

function Where() {
  const at = useLocation();
  return <p data-testid="where">{at.pathname + at.search}</p>;
}

function open(at = "/integrations/credentials", servers = [plain, signIn]) {
  return render(
    <MemoryRouter initialEntries={[at]}>
      <Routes>
        <Route path="/integrations/credentials" element={<>
          <UserCredentialsPanel servers={servers} recipes={[]} credentials={[]} isLoading={false} error={null} onRetry={vi.fn()} />
          <Where />
        </>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("connecting in the browser", () => {
  beforeEach(() => {
    vi.mocked(toast.success).mockReset();
    vi.mocked(toast.error).mockReset();
    api.start.mockReset();
    api.go.mockReset();
    api.pending = false;
  });

  it("says the sign-in is opening while it begins, and cannot be started twice", () => {
    api.pending = true;
    open("/integrations/credentials?server=dpg");
    expect(screen.getByRole("button", { name: "Abrindo o login…" })).toBeDisabled();
  });

  it("begins the sign-in and sends the browser to the provider", async () => {
    api.start.mockResolvedValue({ authorizeURL: "https://idp.example.com/auth?state=s" });
    open("/integrations/credentials?server=dpg");
    await userEvent.click(screen.getByRole("button", { name: "Conectar pelo navegador" }));
    expect(api.start).toHaveBeenCalledWith("dpg");
    expect(api.go).toHaveBeenCalledWith("https://idp.example.com/auth?state=s");
  });

  it("says why the sign-in could not begin, and lets it be tried again", async () => {
    api.start.mockRejectedValue(new Error("boom"));
    open("/integrations/credentials?server=dpg");
    const button = screen.getByRole("button", { name: "Conectar pelo navegador" });
    await userEvent.click(button);
    expect(toast.error).toHaveBeenCalled();
    expect(api.go).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Conectar pelo navegador" })).toBeEnabled();
  });

  it("is offered only for a server people sign in to", () => {
    open("/integrations/credentials?server=alpha");
    expect(screen.queryByRole("button", { name: "Conectar pelo navegador" })).not.toBeInTheDocument();
  });

  it("says the outcome once, on the server it was for, and clears it from the address", () => {
    open("/integrations/credentials?oauth=connected&server=dpg");
    expect(toast.success).toHaveBeenCalledWith("Conectado. Seus agentes já chamam dpg como você.");
    expect(screen.getByRole("heading", { name: "dpg" })).toBeInTheDocument();
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/integrations\/credentials$/);
  });

  it("says a refusal as one", () => {
    open("/integrations/credentials?oauth=not_yours");
    expect(toast.error).toHaveBeenCalledWith("Esse login foi iniciado por outra pessoa. Comece de novo com a sua conta.");
  });

  // The last server can go while its sign-in is in flight: the return still
  // says why, on a page that now lists nothing, and still leaves the address.
  it("says the outcome even when no server is left to list", () => {
    open("/integrations/credentials?oauth=changed&server=dpg", []);
    expect(toast.error).toHaveBeenCalledWith("A configuração do servidor mudou durante o login. Comece de novo.");
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/integrations\/credentials$/);
  });

  // The server says every call takes a personal credential, with no recipe to say so.
  it("tells a server that takes personal credentials apart from one with a fallback", () => {
    open("/integrations/credentials?server=dpg");
    expect(screen.getByText(/A credencial da instalação não pode agir por você/)).toBeInTheDocument();
  });
});
