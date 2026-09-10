import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentApprovalsSection } from "@/features/agents/agent-approvals-section";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";

function renderSection(
  approvals?: AgentDefinition["approvals"],
  answer: (url: string) => Response = eligible,
) {
  const patch = vi.fn();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) =>
      answer(input instanceof Request ? input.url : String(input)),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AgentApprovalsSection
        draft={{ company: "acme", area: "cx", approvals } as AgentDefinition}
        patch={patch}
      />
    </QueryClientProvider>,
  );
  return patch;
}

describe("how an agent's owner asks to be told", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  });

  beforeEach(() => {
    setLocale("pt-BR");
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              items: [
                { id: "usr_ana", display: "Ana" },
                { id: "usr_bob", display: "Bob" },
              ],
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );
  });

  /*
   * An agent that asks for nothing carries no block at all.
   *
   * Written as `{direct: false}` it would grow an approvals section in a file
   * its author never typed, and two publications of one definition would differ —
   * the version is the file's digest.
   */
  it("asks for nothing by leaving the policy absent", async () => {
    const user = userEvent.setup();
    const patch = renderSection({ direct: true, notify: [] });

    await user.click(screen.getByRole("checkbox"));

    expect(patch).toHaveBeenCalledWith({ approvals: undefined });
  });

  it("asks for a private message without naming anybody", async () => {
    const user = userEvent.setup();
    const patch = renderSection();

    await user.click(screen.getByRole("checkbox"));

    expect(patch).toHaveBeenCalledWith({
      approvals: { direct: true, notify: [] },
    });
  });

  // Naming somebody is offered only once a message was asked for: a list of
  // people to message, with no message, is a control that does nothing.
  it("offers the list only when a message was asked for", () => {
    renderSection();

    expect(screen.queryByText("Avisar em particular")).not.toBeInTheDocument();
  });
});

/*
 * The list is the one the fan-out will use.
 *
 * The administrative directory needs authority over identity, which an author
 * publishing an agent does not have: it answered 403 and drew an empty select
 * with no error and no retry — a control that looks like an installation with
 * no approvers in it.
 */
describe("the people an owner may name", () => {
  it("is read from the agent's own scope, not from the directory", async () => {
    const asked: string[] = [];
    renderSection({ direct: true, notify: [] }, (url) => {
      asked.push(url);
      return eligible();
    });

    await screen.findByText("Avisar em particular");
    expect(asked.some((url) => url.includes("/agents/approvers"))).toBe(true);
    expect(asked.some((url) => url.includes("/admin/people"))).toBe(false);
  });

  /*
   * And not read at all until a message is asked for.
   *
   * Awaited rather than asserted straight after rendering: the query runs in
   * an effect, so a synchronous check passes whether or not the hook is
   * enabled — which is what it did, and the sabotage that should have caught it
   * went through.
   */
  it("is not read while the message is switched off", async () => {
    const asked: string[] = [];
    renderSection(undefined, (url) => {
      asked.push(url);
      return eligible();
    });

    await screen.findByText("Aprovação humana");
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(asked).toHaveLength(0);
  });

  it("says why it is empty when the read fails, and offers to try again", async () => {
    renderSection(
      { direct: true, notify: [] },
      () =>
        new Response(JSON.stringify({ title: "não foi possível ler" }), {
          status: 500,
          headers: { "Content-Type": "application/problem+json" },
        }),
    );

    expect(
      await screen.findByRole("button", { name: /Tentar de novo/ }),
    ).toBeEnabled();
  });

  /*
   * The twenty-first name is refused when somebody publishes — minutes later,
   * on another screen. The ceiling is said where the choice is made.
   */
  it("stops at the number the fan-out stops at", async () => {
    const twenty = Array.from({ length: 20 }, (_, i) => `usr_${i}`);
    renderSection({ direct: true, notify: twenty });

    expect(
      await screen.findByText(/o máximo que um pedido alcança/),
    ).toBeInTheDocument();
    expect(screen.getByRole("combobox")).toBeDisabled();
  });
});

function eligible() {
  return new Response(
    JSON.stringify({
      items: [
        { id: "usr_ana", display: "Ana" },
        { id: "usr_bob", display: "Bob" },
      ],
    }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}
