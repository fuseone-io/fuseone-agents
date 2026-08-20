import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RunOutcomeBody } from "@/features/runs/run-outcome-body";

describe("a run's closing answer", () => {
  it("reads as the document the model wrote", () => {
    render(<RunOutcomeBody body={"**Causa raiz**\n\n- token ausente"} />);

    expect(screen.getByText("Causa raiz").tagName).toBe("STRONG");
    expect(screen.getByRole("listitem")).toHaveTextContent("token ausente");
  });

  it("shows a link as text instead of something to click", () => {
    // The answer restates what the agent read, and what it read came from
    // outside. A console that turned that into a clickable link would be
    // offering an auditor a destination a third party chose.
    render(<RunOutcomeBody body="ver [o painel](https://example.invalid/x)" />);

    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText(/https:\/\/example\.invalid\/x/)).toBeInTheDocument();
  });

  it("does not fetch an image the answer names", () => {
    const { container } = render(
      <RunOutcomeBody body="![pixel](https://example.invalid/p.png)" />,
    );

    expect(container.querySelector("img")).toBeNull();
  });

  it("does not interpret markup in the answer", () => {
    const { container } = render(
      <RunOutcomeBody body="antes <img src=x onerror=alert(1)> depois" />,
    );

    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText(/onerror/)).toBeInTheDocument();
  });
});
