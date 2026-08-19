import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CompaniesPanel } from "@/features/companies/companies-panel";
import type { Company } from "@/features/companies/api";

const hooks = vi.hoisted(() => ({
  companies: [] as Company[],
  refetch: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
}));

vi.mock("@/features/companies/api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/companies/api")>();
  return {
    ...actual,
    useCompanies: () => ({
      data: { items: hooks.companies },
      isLoading: false,
      error: null,
      refetch: hooks.refetch,
    }),
    useCreateCompany: () => ({
      mutateAsync: hooks.create,
      isPending: false,
    }),
    useUpdateCompany: () => ({
      mutate: hooks.update,
      mutateAsync: hooks.update,
      isPending: false,
    }),
  };
});

describe("company administration", () => {
  beforeEach(() => {
    hooks.companies = [
      { id: "cora", label: "Cora", areas: 2, archived: false },
      { id: "legacy", label: "Legacy", areas: 1, archived: true },
    ];
    hooks.refetch.mockReset();
    hooks.create.mockReset();
    hooks.update.mockReset();
    hooks.update.mockResolvedValue(undefined);
  });

  it("filters companies with the same toolbar pattern as people", async () => {
    render(<CompaniesPanel />);

    expect(screen.getByRole("searchbox")).toBeInTheDocument();
    expect(screen.getByText("Cora")).toBeInTheDocument();
    expect(screen.getByText("Legacy")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Retiradas" }));
    expect(screen.queryByText("Cora")).not.toBeInTheDocument();
    expect(screen.getByText("Legacy")).toBeInTheDocument();

    await userEvent.type(screen.getByRole("searchbox"), "missing");
    expect(screen.getByText("Nenhuma empresa encontrada")).toBeInTheDocument();
  });

  it("edits the displayed name without changing the identifier", async () => {
    render(<CompaniesPanel />);

    await userEvent.click(screen.getByRole("button", { name: /Cora/ }));
    await userEvent.click(screen.getByRole("button", { name: "Editar" }));
    expect(screen.getByLabelText("Identificador")).toBeDisabled();

    await userEvent.clear(screen.getByLabelText("Como aparece"));
    await userEvent.type(screen.getByLabelText("Como aparece"), "Cora Labs");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(hooks.update).toHaveBeenCalledWith({
      company: "cora",
      label: "Cora Labs",
    });
  });
});
