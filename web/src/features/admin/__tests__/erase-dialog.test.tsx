import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EraseDialog } from "@/features/admin/erase-dialog";

const hooks = vi.hoisted(() => ({
  mutate: vi.fn(),
  onClose: vi.fn(),
}));

vi.mock("@/features/admin/retention-api", () => ({
  useEraseContent: () => ({ mutate: hooks.mutate, isPending: false }),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe("erase dialog", () => {
  beforeEach(() => {
    hooks.mutate.mockReset();
    hooks.onClose.mockReset();
  });

  it("uses an alert dialog for the irreversible erase action", () => {
    render(<EraseDialog onClose={hooks.onClose} />);

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  it("submits the erasure without closing before the mutation succeeds", async () => {
    const user = userEvent.setup();

    render(<EraseDialog onClose={hooks.onClose} />);

    await user.type(screen.getByLabelText("Execuções"), "run_1");
    await user.type(screen.getByLabelText("Motivo"), "pedido do titular");
    await user.click(screen.getByRole("button", { name: "Apagar 1 execução" }));

    expect(hooks.mutate).toHaveBeenCalledWith(
      { runs: ["run_1"], reason: "pedido do titular" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    expect(hooks.onClose).not.toHaveBeenCalled();
  });
});
