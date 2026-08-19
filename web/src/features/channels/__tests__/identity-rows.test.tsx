import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { IdentityRows } from "@/features/channels/identity-rows";

/*
A binding the runtime refuses is shown as refused.

The row that cannot be read has no principal, so rendered as an ordinary one it
reads as somebody with a blank name — and the operator who arrived here because
the platform complained compares that error against a line that looks fine. It
is the same asymmetry the backend just closed, kept alive by the screen.
*/

function show(
  identities: Parameters<typeof IdentityRows>[0]["identities"],
  seenAccounts: Parameters<typeof IdentityRows>[0]["seenAccounts"] = [],
) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <IdentityRows
        channel="acme-slack"
        identities={identities}
        seenAccounts={seenAccounts}
      />
    </QueryClientProvider>,
  );
}

describe("who can decide", () => {
  it("says a binding is unreadable rather than showing a blank name", () => {
    show([{ account: "U404", principal: "", unreadable: true }]);

    expect(screen.getByText(/ilegível/i)).toBeInTheDocument();
  });

  it("shows an ordinary binding by the name of the person", () => {
    show([{ account: "U024", principal: "usr_ana", display: "Ana" }]);

    expect(screen.getByText("Ana")).toBeInTheDocument();
  });

  // Removing it is why it is listed at all: the account is what the button
  // sends, and it survives on a row whose value did not.
  it("still offers to remove a broken one", () => {
    show([{ account: "U404", principal: "", unreadable: true }]);

    expect(screen.getByText("U404")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /remover/i })).toHaveLength(1);
  });

  it("uses a seen account as a form hint rather than a binding", () => {
    show([], [
      {
        account: "U777",
        conversation: "C-alerts",
        lastSeen: "2026-08-19T12:00:00.000Z",
      },
    ]);

    fireEvent.click(screen.getByRole("button", { name: /U777/i }));

    expect(screen.getByLabelText(/conta/i)).toHaveValue("U777");
    expect(screen.queryByText(/vinculado/i)).not.toBeInTheDocument();
  });
});
