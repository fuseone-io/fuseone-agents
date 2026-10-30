import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthLayout } from "@/features/session/auth-layout";
import { BrandingProvider } from "@/features/branding/branding-provider";

const uploaded = `/branding/assets/${"c".repeat(64)}`;

function serveBranding(body: unknown) {
  vi.stubGlobal("fetch", async (input: Request | string) => {
    const url = typeof input === "string" ? input : input.url;
    const path = new URL(url, "http://localhost").pathname;
    const found = path === "/api/v1/branding";
    return new Response(JSON.stringify(found ? body : {}), {
      status: found ? 200 : 404,
      headers: { "Content-Type": "application/json" },
    });
  });
}

afterEach(() => vi.unstubAllGlobals());

describe("the sign-in screen's brand", () => {
  it("loads an uploaded logo from this installation, not from outside", async () => {
    serveBranding({ displayName: "Acme Agents", logoUrl: uploaded });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={client}>
        <BrandingProvider>
          <AuthLayout icon={null} title="Entrar" description="">
            <p>form</p>
          </AuthLayout>
        </BrandingProvider>
      </QueryClientProvider>,
    );

    const logo = await screen.findByRole("img", { name: "Acme Agents" });
    expect(logo).toHaveAttribute("src", uploaded);
  });
});
