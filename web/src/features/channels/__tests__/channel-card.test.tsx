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
      scope: { company: "cora", area: "platform" },
      mode: "mentions",
      wants: ["parked", "failed"],
      enabled: true,
    };
    const edit = renderCard({
      name: "cora-slack",
      kind: "slack",
      workspace: "Cora",
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
        name: "cora-slack",
        kind: "slack",
        workspace: "Cora",
        deliveryMode: "socket",
        enabled: true,
        hasCredential: true,
        hasAppToken: false,
        conversations: [
          {
            id: "C-alerts",
            label: "#alerts",
            scope: { company: "cora", area: "platform" },
            mode: "mentions",
            wants: ["failed"],
            enabled: true,
          },
          {
            id: "C-approvals",
            label: "#approvals",
            scope: { company: "cora", area: "platform" },
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
