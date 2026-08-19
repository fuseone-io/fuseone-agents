import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AreasPanel } from "@/features/admin/areas-panel";
import type { RegisteredScope } from "@/features/scope/api";
import type { Company } from "@/features/companies/api";
import type { Me } from "@/features/session/api";

const hooks = vi.hoisted(() => ({
  areas: [] as RegisteredScope[],
  companies: [] as Company[],
  me: null as Me | null,
  refetch: vi.fn(),
  register: vi.fn(),
  remove: vi.fn(),
}));

vi.mock("@/features/scope/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/scope/api")>();
  return {
    ...actual,
    useScopes: () => ({
      data: { items: hooks.areas },
      isLoading: false,
      error: null,
      refetch: hooks.refetch,
    }),
    useRegisterScope: () => ({
      mutateAsync: hooks.register,
      isPending: false,
    }),
    useDeleteScope: () => ({ mutate: hooks.remove, isPending: false }),
  };
});

vi.mock("@/features/companies/api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/companies/api")>();
  return {
    ...actual,
    useCompanies: () => ({
      data: { items: hooks.companies },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    }),
  };
});

vi.mock("@/features/session/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/session/api")>();
  return { ...actual, useMe: () => ({ data: hooks.me }) };
});

describe("areas administration", () => {
  beforeEach(() => {
    hooks.areas = [
      { company: "cora", area: "support", label: "Support" },
      { company: "acme", area: "finance", label: "Finance" },
    ];
    hooks.companies = [
      { id: "acme", label: "Acme", areas: 1, archived: false },
      { id: "cora", label: "Cora", areas: 1, archived: false },
    ];
    hooks.me = {
      id: "usr_admin",
      display: "Admin",
      kind: "user",
      grants: [{ company: "*", area: "", role: "admin" }],
      can: ["company:write", "scope:write"],
    };
    hooks.register.mockReset();
    hooks.register.mockResolvedValue({
      company: "acme",
      area: "risk",
      label: "Risk",
    });
    hooks.remove.mockReset();
  });

  it("creates an area in a real company instead of the installation wildcard", async () => {
    render(<AreasPanel />);

    await userEvent.click(screen.getByRole("button", { name: "Nova área" }));
    await userEvent.type(screen.getByLabelText("Nome"), "Risk");
    await userEvent.click(screen.getByRole("button", { name: "Declarar" }));

    expect(hooks.register).toHaveBeenCalledWith({
      company: "acme",
      name: "Risk",
      label: undefined,
    });
    expect(screen.queryByText("*")).not.toBeInTheDocument();
  });

  it("filters and relabels the existing area", async () => {
    render(<AreasPanel />);

    await userEvent.type(screen.getByRole("searchbox"), "finance");
    expect(screen.queryByText("Support")).not.toBeInTheDocument();
    expect(screen.getByText("Finance")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Finance/ }));
    await userEvent.click(screen.getByRole("button", { name: "Editar área" }));
    await userEvent.clear(screen.getByLabelText("Como aparece"));
    await userEvent.type(screen.getByLabelText("Como aparece"), "Financeiro");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(hooks.register).toHaveBeenLastCalledWith({
      company: "acme",
      name: "finance",
      label: "Financeiro",
    });
  });
});
