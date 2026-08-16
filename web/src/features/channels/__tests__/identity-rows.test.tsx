import { render, screen } from "@testing-library/react";
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

function show(identities: Parameters<typeof IdentityRows>[0]["identities"]) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <IdentityRows channel="acme-slack" identities={identities} />
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
});
