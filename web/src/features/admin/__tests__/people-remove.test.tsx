import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pathOf } from "@/test/scope-lists-routes";
import type { Person } from "@/features/admin/people-api";
import { PeoplePanel } from "@/features/admin/people-panel";

afterEach(() => vi.unstubAllGlobals());

const person = (over: Partial<Person>): Person => ({
  id: "usr_x",
  kind: "user",
  display: "X",
  disabled: false,
  grants: [],
  ...over,
});

const PEOPLE: Person[] = [
  person({ id: "usr_me", display: "Ana Admin", provider: "local", username: "ana" }),
  person({ id: "usr_bia", display: "Bia Local", provider: "local", username: "bia" }),
  person({ id: "usr_first", display: "Primeiro", provider: "bootstrap", username: "first" }),
  person({ id: "usr_caio", display: "Caio Provider", provider: "oidc:keycloak" }),
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": `application/${status < 300 ? "" : "problem+"}json` },
  });

/** The people routes as the server keeps them: a removed person leaves the list. */
function stub(refusal?: { type: string; detail: string }) {
  let people = PEOPLE;
  const deleted: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string) => {
      const path = pathOf(input);
      if (path === "/me") return json({ id: "usr_me", display: "Ana Admin" });
      if (path === "/admin/people") return json({ items: people });
      deleted.push(path);
      if (refusal) return json({ title: "Conflict", status: 409, ...refusal }, 409);
      people = people.filter((p) => `/admin/people/${p.id}` !== path);
      return new Response(null, { status: 204 });
    }),
  );
  return deleted;
}

async function open(name: string) {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <PeoplePanel />
    </QueryClientProvider>,
  );
  await userEvent.click(await screen.findByRole("button", { name: new RegExp(name) }));
}

describe("removing a person", () => {
  it.each(["Bia Local", "Primeiro"])("is offered for %s, an account made here", async (name) => {
    stub();
    await open(name);
    expect(await screen.findByRole("button", { name: "Remover" })).toBeInTheDocument();
  });

  it.each(["Caio Provider", "Ana Admin"])("is not offered for %s", async (name) => {
    stub();
    await open(name);
    await screen.findByRole("button", { name: "Gerenciar" });
    // Wait for who is signed in, so the absence is not just the session still loading.
    await waitFor(() => expect(fetch).toHaveBeenCalledWith("/api/v1/me", expect.anything()));
    expect(screen.queryByRole("button", { name: "Remover" })).toBeNull();
  });

  it("asks first, naming the person, then removes them and the list refreshes", async () => {
    const deleted = stub();
    await open("Bia Local");
    await userEvent.click(await screen.findByRole("button", { name: "Remover" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Remover Bia Local?" });
    expect(deleted).toEqual([]);
    await userEvent.click(within(dialog).getByRole("button", { name: "Remover" }));

    await waitFor(() => expect(screen.queryByText("Bia Local")).toBeNull());
    expect(deleted).toEqual(["/admin/people/usr_bia"]);
  });

  it("sends nothing when the confirmation is cancelled", async () => {
    const deleted = stub();
    await open("Bia Local");
    await userEvent.click(await screen.findByRole("button", { name: "Remover" }));
    await userEvent.click(await screen.findByRole("button", { name: "Cancelar" }));

    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(deleted).toEqual([]);
  });

  it("says why the server refused and keeps the person listed", async () => {
    stub({ type: "fuseone:last-administrator", detail: "usr_bia" });
    await open("Bia Local");
    await userEvent.click(await screen.findByRole("button", { name: "Remover" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Remover" }));

    expect(await screen.findByText(/Ninguém mais conseguiria gerenciar as pessoas/)).toBeInTheDocument();
    expect(screen.getAllByText("Bia Local").length).toBeGreaterThan(0);
  });
});
