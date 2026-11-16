import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { Table, TableBody } from "@/components/ui/table";
import { ConversationRow } from "@/features/channels/conversation-row";
import type { Conversation } from "@/features/channels/channel-model";
import { sessionKeys, type Me } from "@/features/session/api";
import { setLocale } from "@/i18n";

// NT-019 phase 4: on a connection an area inherits, the company's room is
// shown to the area's administrator for reading, and nothing they would be
// refused is offered: no edit, test or removal.

const areaAdmin = { id: "usr_cx", display: "Cx", kind: "human", can: ["provider:write"], platform: [], grants: [], configure: [{ company: "acme", area: "cx" }] } as unknown as Me;

function rowAt(scope: { company: string; area?: string }) {
  const client = new QueryClient();
  client.setQueryData(sessionKeys.me, areaAdmin);
  render(
    <QueryClientProvider client={client}>
      <Table>
        <TableBody>
          <ConversationRow channel="acme-slack" onEdit={() => {}}
            conversation={{ id: "C1", label: "#room", scope, mode: "announce", enabled: true } as Conversation} />
        </TableBody>
      </Table>
    </QueryClientProvider>,
  );
}

beforeEach(() => setLocale("pt-BR"));

describe("a conversation's actions", () => {
  it("are offered on a room of the area its administrator configures", () => {
    rowAt({ company: "acme", area: "cx" });

    expect(screen.getByRole("button", { name: "Editar conversa" })).toBeInTheDocument();
  });

  it.each([{ company: "acme" }, { company: "*" }])("are not offered on a room of %o they only read", (scope) => {
    rowAt(scope);

    for (const name of ["Editar conversa", "Enviar teste", "Remover"]) {
      expect(screen.queryByRole("button", { name }), name).not.toBeInTheDocument();
    }
  });
});
