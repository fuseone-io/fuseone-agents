import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { ManualBody } from "@/features/manual/manual-body";

function show(body: string) {
  return render(
    <MemoryRouter>
      <ManualBody body={body} />
    </MemoryRouter>,
  );
}

describe("a page of the manual", () => {
  it("sends an internal link to the console route, not to a file", () => {
    // The manual is one text read in two places. On GitHub the link is a
    // path; here it has to be a route, or every cross-reference is a 404 in
    // the console while looking correct in the pull request.
    show("Ver [o que para](what-the-gate-stops.md) antes.");

    expect(screen.getByRole("link", { name: "o que para" })).toHaveAttribute(
      "href",
      "/manual/what-the-gate-stops",
    );
  });

  it("does not render markup somebody put in the text", () => {
    // No raw-HTML plugin is enabled, which is the whole safety story: markup
    // is not sanitised after being parsed, it is never parsed as markup.
    const { container } = show("Antes <img src=x onerror=alert(1)> depois.");

    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText(/onerror/)).toBeInTheDocument();
  });

  it("opens an external link away from the console", () => {
    show("Ver [o repositório](https://github.com/fuseone-io/fuseone-agents).");

    const link = screen.getByRole("link", { name: "o repositório" });
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("renders the tables the manual uses to compare things", () => {
    show("| Degrau | O que acontece |\n|---|---|\n| Ler | Passa direto |");

    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Passa direto" })).toBeInTheDocument();
  });
});
