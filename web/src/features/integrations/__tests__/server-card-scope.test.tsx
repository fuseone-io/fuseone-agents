import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { ServerCard } from "@/features/integrations/server-card";
import type { MCPServer } from "@/features/integrations/api";
import { setLocale } from "@/i18n";

// NT-019: a server says where it is stored, and one the caller inherits is
// shown without anything the server would refuse them.

const remote: MCPServer = { name: "crm", transport: "http", url: "https://crm.example.com/mcp", enabled: true };

function card(server: MCPServer) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ServerCard server={server} onEdit={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => setLocale("pt-BR"));

describe("a server card's scope", () => {
  it("names the area a server is stored at", () => {
    card({ ...remote, scopeKind: "area", company: "acme", area: "cx", editable: true });

    expect(screen.getByText("acme/cx")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar" })).toBeInTheDocument();
  });

  it("names the installation for a server stored there", () => {
    card({ ...remote, scopeKind: "installation", editable: true });

    expect(screen.getByText("Instalação")).toBeInTheDocument();
  });

  it("offers nothing to change on a server the caller inherits", () => {
    card({ ...remote, scopeKind: "installation", editable: false });

    expect(screen.getByText("Da plataforma")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remover crm?" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Ferramentas e superfície" })).not.toBeInTheDocument();
  });

  it("keeps every action on a server that does not say whether it is editable", () => {
    card(remote);

    expect(screen.getByRole("button", { name: "Editar" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Ferramentas e superfície" })).toBeInTheDocument();
  });
});
