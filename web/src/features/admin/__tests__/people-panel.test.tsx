import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PeoplePanel } from "@/features/admin/people-panel";
import type { Person } from "@/features/admin/people-api";

const hooks = vi.hoisted(() => ({
  people: [] as Person[],
  refetch: vi.fn(),
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
    grants: [
      {
        company: "cora",
        area: "finance",
        role: "auditor",
        asserted: false,
      },
    ],
    disabled: false,
  },
];

describe("people administration", () => {
  beforeEach(() => {
    hooks.people = people;
    hooks.refetch.mockReset();
  });

  it("filters by identity and access without making the row layout carry the search", async () => {
    render(<PeoplePanel />);

    const search = screen.getByRole("searchbox", { name: "Buscar pessoas" });
    expect(search).toBeInTheDocument();
    expect(screen.getByText("Kleber Rocha")).toBeInTheDocument();
    expect(screen.getByText("Sergio Monteiro")).toBeInTheDocument();
    expect(screen.getAllByText("2 de 2").length).toBeGreaterThan(0);

    await userEvent.type(search, "finance");

    expect(screen.queryByText("Kleber Rocha")).not.toBeInTheDocument();
    expect(screen.getByText("Sergio Monteiro")).toBeInTheDocument();
    expect(screen.getAllByText("1 de 2").length).toBeGreaterThan(0);

    await userEvent.clear(search);
    await userEvent.type(search, "curador");

    expect(screen.getByText("Kleber Rocha")).toBeInTheDocument();
    expect(screen.queryByText("Sergio Monteiro")).not.toBeInTheDocument();
    expect(screen.getAllByText("1 de 2").length).toBeGreaterThan(0);
  });

  it("keeps access, activity and actions as named columns", () => {
    render(<PeoplePanel />);

    expect(screen.getAllByText("Acesso").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Última atividade").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Ações").length).toBeGreaterThan(0);

    const row = screen.getByText("Kleber Rocha").closest("li")
      ?.firstElementChild;
    expect(row?.className).toContain("lg:grid-cols");
    expect(
      within(screen.getByText("Sergio Monteiro").closest("li")!).getByText(
        "nunca visto",
      ),
    ).toBeInTheDocument();
  });
});
