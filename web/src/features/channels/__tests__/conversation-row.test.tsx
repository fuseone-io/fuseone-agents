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
            conversation={
              {
                id: "C-alerts",
                label: "#alerts",
                scope: { company: "acme", area: "devops" },
                mode: "mentions",
                wants: ["parked"],
                enabled: true,
                ...conversation,
              } as Conversation
            }
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

// A conversation that also messages people privately says so in the listing.
// It is a decision about who sees a run's facts, and a screen that showed the
// channel card and not the private one would understate what is configured.
describe("private approvals in the listing", () => {
  it("says when approvals also go out privately", () => {
    renderRow({ mode: "mentions", directApprovals: true });

    expect(screen.getByText(/dm/)).toBeInTheDocument();
  });

  it("says nothing when they do not", () => {
    renderRow({ mode: "mentions" });

    expect(screen.queryByText(/dm/)).not.toBeInTheDocument();
  });
});

/*
 * The scope above every company is stored as the company "*".
 *
 * A listing that printed it raw would show a company nobody has, next to a
 * mode read as "mentions" — two lies about the one conversation that hears
 * every company and starts nothing.
 */
describe("a conversation for the whole installation", () => {
  it("is named rather than printed as the company that stores it", () => {
    renderRow({ scope: { company: "*", area: "" }, mode: "announce" });

    expect(screen.getByText("A instalação inteira")).toBeInTheDocument();
    expect(screen.queryByText("*")).not.toBeInTheDocument();
  });

  it("says it only reports", () => {
    renderRow({ scope: { company: "*", area: "" }, mode: "announce" });

    expect(screen.getByText(/Só relata/)).toBeInTheDocument();
  });
});

// And the listing says what is stored rather than the nearest thing it knows.
// "Menções" beside a mode nothing can name would claim the room starts runs,
// which is the one thing an unknown mode is guaranteed not to do.
describe("a mode this console does not know", () => {
  it("is printed as it is stored", () => {
    renderRow({ mode: "a-future-mode" });

    expect(screen.getByText(/a-future-mode/)).toBeInTheDocument();
  });
});

// An event this console cannot translate is printed as it is stored. Passed to
// t() it renders as "channels.event.a-future-event", which tells a reader about
// our key naming and nothing about their configuration.
describe("an event this console does not know", () => {
  it("is printed as it is stored", () => {
    renderRow({ mode: "mentions", wants: ["parked", "a-future-event"] });

    expect(screen.getByText("a-future-event")).toBeInTheDocument();
  });
});
