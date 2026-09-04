import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Table, TableBody } from "@/components/ui/table";
import { ConversationRow } from "@/features/channels/conversation-row";
import type { Conversation } from "@/features/channels/channel-model";

function renderRow(conversation: Partial<Conversation>) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Table>
        <TableBody>
          <ConversationRow
            channel="acme-slack"
            conversation={{
              id: "C-alerts",
              label: "#alerts",
              scope: { company: "acme", area: "devops" },
              mode: "mentions",
              wants: ["parked"],
              enabled: true,
              ...conversation,
            } as Conversation}
            onEdit={() => {}}
          />
        </TableBody>
      </Table>
    </QueryClientProvider>,
  );
}

describe("a conversation in the listing", () => {
  // The binding decides which agent a mention starts, so a listing that showed
  // it only for watched messages would hide the very thing somebody configured.
  it("names the bound agent of a conversation that only takes mentions", () => {
    renderRow({ mode: "mentions", agent: "troubleshooting-sre" });

    expect(screen.getByText(/troubleshooting-sre/)).toBeInTheDocument();
  });

  it("says nothing about an agent where nobody bound one", () => {
    renderRow({ mode: "mentions" });

    expect(screen.queryByText(/·/)).not.toBeInTheDocument();
  });
});
