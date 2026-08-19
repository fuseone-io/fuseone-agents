import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StopButton } from "@/features/admin/stop-button";

const hooks = vi.hoisted(() => ({
  mutate: vi.fn(),
  me: {
    id: "usr_ana",
    display: "Ana",
    kind: "human",
    can: ["run:read"],
    grants: [{ company: "acme", area: "devops", role: "approver" }],
  },
  scopes: {
    items: [
      { company: "acme", area: "devops", label: "DevOps" },
      { company: "acme", area: "finance", label: "Finance" },
    ],
  },
}));

vi.mock("@/features/session/api", () => ({
  useMe: () => ({ data: hooks.me }),
}));

vi.mock("@/features/admin/stops-api", () => ({
  useStops: () => ({ data: [] }),
  useSetStop: () => ({ mutate: hooks.mutate, isPending: false }),
}));

vi.mock("@/features/scope/api", () => ({
  useScopes: () => ({ data: hooks.scopes }),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe("stop button", () => {
  beforeEach(() => {
    hooks.mutate.mockReset();
  });

  it("stops the reachable area rather than defaulting to the whole installation", async () => {
    const user = userEvent.setup();

    render(<StopButton />);

    await user.click(screen.getByRole("button", { name: "Parar" }));
    await user.type(screen.getByLabelText("Por quê"), "provider outage");
    await user.click(screen.getAllByRole("button", { name: "Parar" }).at(-1)!);

    expect(hooks.mutate).toHaveBeenCalledWith(
      {
        level: "scope",
        stopped: true,
        reason: "provider outage",
        scope: { company: "acme", area: "devops" },
      },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });
});
