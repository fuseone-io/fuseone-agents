import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PeoplePanel } from "@/features/admin/people-panel";
import type { Person } from "@/features/admin/people-api";

const hooks = vi.hoisted(() => ({
  people: [] as Person[],
  refetch: vi.fn(),
  setGrants: vi.fn(),
}));

vi.mock("@/features/admin/people-api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/admin/people-api")>();
  return {
    ...actual,
    usePeople: () => ({
      data: { items: hooks.people },
      isLoading: false,
      error: null,
      refetch: hooks.refetch,
    }),
    useSetGrants: () => ({ mutate: hooks.setGrants, isPending: false }),
  };
});

const people: Person[] = [
  {
    id: "usr_kleber",
    kind: "user",
    display: "Kleber Rocha",
    email: "kleber@example.com",
    provider: "oidc:keycloak",
    grants: [
      {
        company: "cora",
        area: "devops",
        role: "curator",
        asserted: true,
      },
    ],
    lastSeen: "2026-08-19T02:00:00.000Z",
    disabled: false,
  },
  {
    id: "usr_sergio",
    kind: "user",
    display: "Sergio Monteiro",
    email: "sergio@example.com",
    provider: "oidc:keycloak",
    grants: [
      {
        company: "cora",
        area: "finance",
        role: "approver",
        asserted: false,
      },
      {
        company: "cora",
        area: "finance",
        role: "auditor",
        asserted: false,
      },
      {
        company: "cora",
        area: "finance",
        role: "author",
        asserted: false,
      },
      {
        company: "cora",
        area: "finance",
        role: "curator",
        asserted: false,
      },
    ],
    disabled: false,
  },
  {
    id: "usr_devops",
    kind: "user",
    display: "Devops",
    email: "devops@example.com",
    username: "devops",
    grants: [],
    disabled: false,
  },
];

describe("people administration", () => {
  beforeEach(() => {
    hooks.people = people;
    hooks.refetch.mockReset();
    hooks.setGrants.mockReset();
  });

  it("filters by identity and access without making the row layout carry the search", async () => {
    render(<PeoplePanel />);

    const search = screen.getByRole("searchbox", { name: "Buscar pessoas" });
    expect(search).toBeInTheDocument();
    expect(screen.getByText("Kleber Rocha")).toBeInTheDocument();
    expect(screen.getByText("Sergio Monteiro")).toBeInTheDocument();
    expect(screen.getAllByText("3 de 3 pessoas").length).toBeGreaterThan(0);

    await userEvent.type(search, "finance");

    expect(screen.queryByText("Kleber Rocha")).not.toBeInTheDocument();
    expect(screen.getByText("Sergio Monteiro")).toBeInTheDocument();
    expect(screen.getAllByText("1 de 3 pessoas").length).toBeGreaterThan(0);

    await userEvent.clear(search);
    await userEvent.type(search, "kleber@example");

    expect(screen.getByText("Kleber Rocha")).toBeInTheDocument();
    expect(screen.queryByText("Sergio Monteiro")).not.toBeInTheDocument();
    expect(screen.getAllByText("1 de 3 pessoas").length).toBeGreaterThan(0);
  });

  it("groups access by scope and opens the role matrix for detail", async () => {
    render(<PeoplePanel />);

    expect(screen.getAllByText("Acesso").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Entrada").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Visto").length).toBeGreaterThan(0);
    expect(screen.getByText("cora/finance")).toBeInTheDocument();
    expect(screen.getByText("acesso completo")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: /Sergio Monteiro/ }),
    );

    const matrix = screen.getByRole("table");
    expect(screen.getByText("Papéis por escopo")).toBeInTheDocument();
    expect(
      within(screen.getByText("Sergio Monteiro").closest("li")!).getByText(
        "nunca visto",
      ),
    ).toBeInTheDocument();
    expect(within(matrix).getByText("Aprovador")).toBeInTheDocument();
    expect(within(matrix).getByText("Administrador")).toBeInTheDocument();
    expect(within(matrix).getByText("Auditor")).toBeInTheDocument();
    expect(within(matrix).getByText("Autor")).toBeInTheDocument();
    expect(within(matrix).getByText("Curador")).toBeInTheDocument();
    expect(within(matrix).getByText("concedido aqui")).toBeInTheDocument();
  });

  it("offers an installation administrator grant without making the operator type the wildcard", async () => {
    render(<PeoplePanel />);

    await userEvent.click(
      screen.getByRole("button", { name: /Kleber Rocha/ }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Gerenciar" }));
    await userEvent.click(
      screen.getByRole("button", { name: "Conceder administrador" }),
    );

    expect(screen.getByDisplayValue("*")).toBeInTheDocument();
    const roles = screen.getAllByRole("combobox", { name: "Papel" });
    expect(roles.at(-1)).toHaveTextContent("Administrador");
  });

  it("filters the list by sign-in source and missing roles", async () => {
    render(<PeoplePanel />);

    await userEvent.click(screen.getByRole("button", { name: "Provedor" }));
    expect(screen.getByText("Kleber Rocha")).toBeInTheDocument();
    expect(screen.getByText("Sergio Monteiro")).toBeInTheDocument();
    expect(screen.queryByText("Devops")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Local" }));
    expect(screen.queryByText("Kleber Rocha")).not.toBeInTheDocument();
    expect(screen.queryByText("Sergio Monteiro")).not.toBeInTheDocument();
    expect(screen.getByText("Devops")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Sem papel" }));
    expect(screen.queryByText("Kleber Rocha")).not.toBeInTheDocument();
    expect(screen.queryByText("Sergio Monteiro")).not.toBeInTheDocument();
    expect(screen.getByText("Devops")).toBeInTheDocument();
  });
});
