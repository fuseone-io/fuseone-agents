import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Toolbar } from "@/components/shared/toolbar";

describe("the filter bar", () => {
  it("gives the search field a name, because a placeholder disappears on the first keystroke", () => {
    render(<Toolbar placeholder="Buscar por execução" value="" onChange={() => {}} />);
    expect(screen.getByRole("searchbox", { name: "Buscar por execução" })).toBeInTheDocument();
  });

  it("reports what was typed", async () => {
    const onChange = vi.fn();
    render(<Toolbar placeholder="Buscar" value="" onChange={onChange} />);

    await userEvent.type(screen.getByRole("searchbox"), "8801");

    // Controlled: the field reports every keystroke and the page owns the
    // value, which is what lets the search reach the server.
    expect(onChange).toHaveBeenCalledTimes(4);
    expect(onChange).toHaveBeenNthCalledWith(1, "8");
  });
});
