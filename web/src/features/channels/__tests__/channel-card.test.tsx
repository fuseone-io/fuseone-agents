import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChannelCard } from "@/features/channels/channel-card";
import { setLocale } from "@/i18n";
import type { components } from "@/lib/api/schema.gen";

type Channel = components["schemas"]["Channel"];

function renderCard(
  channel: Channel,
  onEditConversation = vi.fn(),
  view: "all" | "attention" | "approvals" = "all",
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <ChannelCard
        channel={channel}
        query=""
        view={view}
        onEdit={() => {}}
        onAddConversation={() => {}}
        onEditConversation={onEditConversation}
      />
    </QueryClientProvider>,
  );
  return onEditConversation;
}

describe("channel card", () => {
  it("offers editing for an existing conversation", async () => {
    setLocale("pt-BR");
    const user = userEvent.setup();
    const conversation: Channel["conversations"][number] = {
      id: "C-alerts",
      label: "#alerts",
      scope: { company: "acme", area: "platform" },
      mode: "mentions",
      wants: ["parked", "failed"],
      enabled: true,
    };
    const edit = renderCard({
      name: "acme-slack",
      kind: "slack",
      workspace: "Acme",
      deliveryMode: "socket",
      enabled: true,
      hasCredential: true,
      hasAppToken: false,
      conversations: [conversation],
    });

    await user.click(screen.getByRole("button", { name: "Editar conversa" }));

    expect(edit).toHaveBeenCalledWith(conversation);
  });

  it("keeps the approvals view focused on conversations that ask people", () => {
    setLocale("en-US");
    renderCard(
      {
        name: "acme-slack",
        kind: "slack",
        workspace: "Acme",
        deliveryMode: "socket",
        enabled: true,
        hasCredential: true,
        hasAppToken: false,
        conversations: [
          {
            id: "C-alerts",
            label: "#alerts",
            scope: { company: "acme", area: "platform" },
            mode: "mentions",
            wants: ["failed"],
            enabled: true,
          },
          {
            id: "C-approvals",
            label: "#approvals",
            scope: { company: "acme", area: "platform" },
            mode: "mentions",
            wants: ["parked"],
            enabled: true,
          },
        ],
      },
      vi.fn(),
      "approvals",
    );

    expect(screen.queryByText("#alerts")).not.toBeInTheDocument();
    expect(screen.getByText("#approvals")).toBeInTheDocument();
  });
});

/*
 * A delivery mode this console cannot name is not a healthy connection.
 *
 * The card read "anything that is not socket" as HTTP, so a connection a newer
 * version configured showed as answering, with a strip offering to bind people
 * to an inbound door the runtime has closed.
 */
describe("a delivery mode this console does not know", () => {
  it("says what is stored and asks for attention", () => {
    setLocale("en-US");
    renderCard({
      name: "acme-slack",
      kind: "slack",
      deliveryMode: "a-future-mode",
      enabled: true,
      hasCredential: true,
      hasSigning: true,
      conversations: [],
    });

    expect(screen.getByText("a-future-mode")).toBeInTheDocument();
    expect(screen.queryByText(/answering/)).not.toBeInTheDocument();
  });

  // Nothing inbound is offered either. A conversation configured there would do
  // nothing today and come into force the day a version that understands the
  // connection reads it, with nobody having decided anything in between.
  it("does not offer to add a conversation", () => {
    setLocale("en-US");
    renderCard({
      name: "acme-slack",
      kind: "slack",
      deliveryMode: "a-future-mode",
      enabled: true,
      hasCredential: true,
      hasSigning: true,
      conversations: [],
    });

    expect(
      screen.queryByRole("button", { name: /Add a conversation/i }),
    ).not.toBeInTheDocument();
  });
});
