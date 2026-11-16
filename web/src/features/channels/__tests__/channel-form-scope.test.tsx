import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ChannelForm } from "@/features/channels/channel-form";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";

const api = vi.hoisted(() => ({ mutateAsync: vi.fn() }));

vi.mock("@/features/channels/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/channels/api")>();
  return { ...actual, useSaveChannel: () => ({ mutateAsync: api.mutateAsync, isPending: false }) };
});
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

beforeEach(() => {
  setLocale("pt-BR");
  useActiveScope.setState({ company: "", area: "" });
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
  api.mutateAsync.mockReset();
  api.mutateAsync.mockResolvedValue(undefined);
});

function sheet(choice?: { anyScope: boolean; configure: { company: string; area: string }[] }) {
  const form = <ChannelForm channel={null} kinds={["slack"]} onClose={vi.fn()} />;
  render(
    <QueryClientProvider client={new QueryClient()}>
      {choice ? <ScopeChoiceContext.Provider value={choice}>{form}</ScopeChoiceContext.Provider> : form}
    </QueryClientProvider>,
  );
}

// NT-019 phase 4: a connection lives at a scope. An area's administrator
// stores it in the area they configure; the platform at the installation.
describe("where a connection is stored", () => {
  it("stores an area administrator's new connection in their area", async () => {
    sheet({ anyScope: false, configure: [{ company: "acme", area: "cx" }] });
    await userEvent.type(screen.getByLabelText("Nome"), "cx-slack");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ scopeKind: "area", company: "acme", area: "cx" }) }),
    );
  });

  it("names the connection's scope in its own words", () => {
    sheet({ anyScope: false, configure: [{ company: "acme", area: "cx" }] });

    expect(screen.getByText("Onde a conexão vale")).toBeInTheDocument();
  });

  it("stores the platform's new connection at the installation by default", async () => {
    sheet();
    await userEvent.type(screen.getByLabelText("Nome"), "shared-slack");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ scopeKind: "installation" }) }),
    );
  });
});
