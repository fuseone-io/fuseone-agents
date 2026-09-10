import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentApprovalsSection } from "@/features/agents/agent-approvals-section";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";

function renderSection(approvals?: AgentDefinition["approvals"]) {
  const patch = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AgentApprovalsSection
        draft={{ approvals } as AgentDefinition}
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
