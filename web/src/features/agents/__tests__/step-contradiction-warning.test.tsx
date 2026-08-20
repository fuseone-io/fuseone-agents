import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { StepContradictionWarning } from "@/features/agents/step-contradiction-warning";

describe("a contradiction between Never and the stages", () => {
  it("shows the stage and opens the place that fixes it", async () => {
    const onOpen = vi.fn();
    render(
      <StepContradictionWarning
        conflicts={[{ at: 0, why: "forbiddenStop", term: "runbook" }]}
        onOpen={onOpen}
      />,
    );

    expect(screen.getByText(/conflito com Nunca/i)).toBeInTheDocument();
    expect(screen.getByText(/passo 1 ainda para por/i)).toBeInTheDocument();
    expect(screen.getByText("runbook")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Revisar passos/ }));

    expect(onOpen).toHaveBeenCalledOnce();
  });
});
