import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ConversationForm } from "@/features/channels/conversation-form";
import { setLocale } from "@/i18n";

const api = vi.hoisted(() => ({ mutateAsync: vi.fn() }));

vi.mock("@/features/channels/api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/channels/api")>();
  return {
    ...actual,
    useSaveConversation: () => ({
      mutateAsync: api.mutateAsync,
      isPending: false,
    }),
    useChannelConversations: () => ({ data: { items: [] } }),
  };
});
vi.mock("@/features/agents/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/agents/api")>();
  return { ...actual, useAgentsForScope: () => ({ data: { items: [] } }) };
});
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

beforeEach(() => {
  setLocale("pt-BR");
  api.mutateAsync.mockReset();
  api.mutateAsync.mockResolvedValue(undefined);
});

describe("finishes that carry the answer", () => {
  // The option is offered only beside the event it narrows: shown without
  // Finished ticked it would be a preference nothing reads.
  it("is offered only when Finished is among the announcements", async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ConversationForm
          channel="acme-slack"
          conversation={undefined}
          onClose={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(
      screen.queryByText("Só anunciar términos que tragam a resposta"),
    ).not.toBeInTheDocument();

    await userEvent.click(screen.getByText("Terminou"));
    expect(
      screen.getByText("Só anunciar términos que tragam a resposta"),
    ).toBeInTheDocument();
  });
});
