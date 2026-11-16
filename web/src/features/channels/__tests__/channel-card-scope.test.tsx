import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { ChannelCard } from "@/features/channels/channel-card";
import { setLocale } from "@/i18n";
import type { components } from "@/lib/api/schema.gen";

// NT-019 phase 4: a connection says where it is stored, and one the caller
// inherits is shown without anything the server would refuse them.

type Channel = components["schemas"]["Channel"];
const base: Channel = { name: "cx-slack", kind: "slack", enabled: true, hasCredential: true, conversations: [] };

function card(channel: Channel) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChannelCard channel={channel} query="" view="all" onEdit={() => {}} onAddConversation={() => {}} onEditConversation={() => {}} />
    </QueryClientProvider>,
  );
}

beforeEach(() => setLocale("pt-BR"));

describe("a connection card's scope", () => {
  it("names the area a connection is stored at, and offers to edit it", () => {
    card({ ...base, scopeKind: "area", company: "acme", area: "cx", editable: true });

    expect(screen.getByText("acme/cx")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar" })).toBeInTheDocument();
  });

  it("shows an inherited connection as the platform's, with nothing to change", () => {
    card({ ...base, scopeKind: "installation", editable: false });

    expect(screen.getByText("Da plataforma")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument();
  });

  // An area may report through the platform's connection, so attaching its own
  // room there stays offered on a connection it only inherits.
  it("keeps offering to add a conversation on an inherited connection", () => {
    card({ ...base, scopeKind: "installation", editable: false });

    expect(screen.getByRole("button", { name: "Adicionar conversa" })).toBeInTheDocument();
  });
});
