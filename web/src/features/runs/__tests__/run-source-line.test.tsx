import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { RunSourceLine } from "@/features/runs/run-source-line";

describe("where a run's input came from", () => {
  it("names the recorded run it was opened from, as a link to it", () => {
    render(
      <MemoryRouter>
        <RunSourceLine sourceRunId="run-1" />
      </MemoryRouter>,
    );

    expect(
      screen.getByText(/aberta a partir da execução/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "run-1" })).toHaveAttribute(
      "href",
      "/runs/run-1",
    );
  });

  it("says nothing about a run nobody opened from another", () => {
    const { container } = render(
      <MemoryRouter>
        <RunSourceLine />
      </MemoryRouter>,
    );

    expect(container).toBeEmptyDOMElement();
  });
});
