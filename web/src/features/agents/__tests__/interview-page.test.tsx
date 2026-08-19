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
  await userEvent.type(
    screen.getByLabelText("Processo"),
    "Ler o chamado, procurar o cliente e responder.",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Revisar como respostas" }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Concluir" }));
}

describe("the agent interview", () => {
  beforeEach(() => {
    interview.mutate.mockReset();
    vi.mocked(toast.error).mockReset();
  });

  it("captures freely first and only sends reviewed answers to the assistant", async () => {
    interview.mutate.mockImplementation((_answers, options) => {
      options.onSuccess({ tools: [], steps: [] });
    });

    renderPage();

    await userEvent.type(
      screen.getByLabelText("Processo"),
      "Quando chega um chamado, procuro o cliente e respondo.",
    );
    expect(interview.mutate).not.toHaveBeenCalled();

    await userEvent.click(
      screen.getByRole("button", { name: "Revisar como respostas" }),
    );
    expect(screen.getByLabelText("Quais são os passos?")).toHaveValue(
      "Quando chega um chamado, procuro o cliente e respondo.",
    );

    await userEvent.click(screen.getByRole("button", { name: "Concluir" }));
    expect(interview.mutate).toHaveBeenCalledWith(
      expect.objectContaining({
        steps: "Quando chega um chamado, procuro o cliente e respondo.",
      }),
      expect.anything(),
    );
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
