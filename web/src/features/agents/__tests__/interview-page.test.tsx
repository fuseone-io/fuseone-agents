import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { InterviewPage } from "@/features/agents/interview-page";
import { ApiError, type Problem } from "@/lib/api/client";
import { toast } from "sonner";

const interview = vi.hoisted(() => ({
  mutate: vi.fn(),
}));

vi.mock("@/features/agents/interview-api", () => ({
  useInterview: () => ({ mutate: interview.mutate, isPending: false }),
}));

vi.mock("sonner", () => ({
  toast: { error: vi.fn() },
}));

function renderPage() {
  return render(
    <MemoryRouter>
      <InterviewPage />
    </MemoryRouter>,
  );
}

async function finishInterview() {
  for (let i = 0; i < 6; i += 1) {
    await userEvent.click(screen.getByRole("button", { name: "Pular" }));
  }
  await userEvent.click(screen.getByRole("button", { name: "Concluir" }));
}

describe("the agent interview", () => {
  beforeEach(() => {
    interview.mutate.mockReset();
    vi.mocked(toast.error).mockReset();
  });

  it("shows the upstream refusal detail instead of only the problem title", async () => {
    const refusal: Problem = {
      type: "fuseone:upstream-refused",
      title: "Upstream refused",
      status: 400,
      detail: "model_provider_overloaded: request id req_123",
    };
    interview.mutate.mockImplementation((_answers, options) => {
      options.onError(new ApiError(400, refusal));
    });

    renderPage();
    await finishInterview();

    expect(toast.error).toHaveBeenCalledWith(
      "O sistema do outro lado recusou: model_provider_overloaded: request id req_123",
    );
  });
});
