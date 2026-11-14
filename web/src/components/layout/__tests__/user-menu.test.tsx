import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SidebarProvider } from "@/components/ui/sidebar";
import { UserMenu } from "@/components/layout/user-menu";

const me = { id: "usr_ana", display: "Ana Lima", kind: "user", grants: [], can: [], platform: [], configure: [] };

function answer(path: string): Response {
  if (path.endsWith("/me")) return Response.json(me);
  if (path.endsWith("/healthz")) return Response.json({ status: "ok", version: "0.110.0" });
  return new Response(null, { status: 404 });
}

describe("the user menu", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("says which version of the platform is running, below the name", async () => {
    vi.stubGlobal("fetch", async (input: Request | string) =>
      answer(new URL(typeof input === "string" ? input : input.url, "http://localhost").pathname));
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <SidebarProvider>
          <UserMenu />
        </SidebarProvider>
      </QueryClientProvider>,
    );
    await userEvent.setup().click(await screen.findByRole("button", { name: /Ana Lima/ }));

    expect(await screen.findByText("Versão 0.110.0")).toBeInTheDocument();
  });
});
