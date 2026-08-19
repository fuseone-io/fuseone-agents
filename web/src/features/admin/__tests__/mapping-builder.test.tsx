import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MappingBuilder } from "@/features/admin/mapping-builder";

describe("identity provider group mappings", () => {
  it("adds an installation administrator mapping without typing the wildcard", async () => {
    const onChange = vi.fn();

    render(<MappingBuilder mappings={[]} onChange={onChange} />);

    await userEvent.click(
      screen.getByRole("button", {
        name: "Adicionar mapeamento de administrador",
      }),
    );

    expect(onChange).toHaveBeenCalledWith([
      { group: "", company: "*", area: "", role: "admin" },
    ]);
  });
});
