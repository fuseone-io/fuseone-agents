import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { ProviderCard } from "@/features/integrations/provider-card";
import type { ModelProvider } from "@/features/integrations/api";
import { setLocale } from "@/i18n";

// NT-019 phase 3: a provider says where it is stored, and one the caller
// inherits is shown without anything the server would refuse them.

const base: ModelProvider = { name: "llm", kind: "openai_compatible", baseUrl: "https://llm.example.com/v1", enabled: true, hasKey: true };

function card(provider: ModelProvider) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <ProviderCard provider={provider} onEdit={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => setLocale("pt-BR"));

describe("a provider card's scope", () => {
  it("names the area a provider is stored at, and offers to edit it", () => {
    card({ ...base, scopeKind: "area", company: "acme", area: "cx", editable: true });

    expect(screen.getByText("acme/cx")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar" })).toBeInTheDocument();
  });

  it("shows an inherited provider as the platform's, with no edit", () => {
    card({ ...base, scopeKind: "installation", editable: false });

    expect(screen.getByText("Da plataforma")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument();
  });
});
