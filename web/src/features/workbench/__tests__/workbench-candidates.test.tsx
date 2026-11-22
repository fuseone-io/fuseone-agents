import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve, type Reply, type Routes } from "@/features/workbench/__tests__/workbench-fixtures";

/*
The workbench's candidates (NT-018 phase 2, release 2d). The saved draft is sealed as a
candidate under its ETag; the candidates the person may see are listed newest first, and
one opens on its sealed text, read from the candidate itself, never from the draft.
*/

const DRAFT = "/agents/triagem/draft";
const SEALS = `${DRAFT}/seals`;
const CANDIDATES = "/agents/triagem/candidates";
const seal = { version: "v2abc", sealId: "s-1", company: "acme", area: "cx", sealedBy: "usr_ana", sealedAt: "2026-11-19T12:00:00Z" };

function world(over: Routes = {}) {
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
    [`GET ${CANDIDATES}`]: () => ({ status: 404 }),
    ...over,
  });
}

async function openCandidates() {
  openAt("/agents/triagem/workbench");
  await userEvent.click(await screen.findByRole("tab", { name: "Candidatas" }));
}

const sealWith = (reply: () => Reply) => ({ [`POST ${SEALS}`]: reply });

describe("the workbench's candidates", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("explains what a candidate is while there is none, and how to make one", async () => {
    world();
    await openCandidates();
    expect(await screen.findByText("Nenhuma candidata ainda")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Selar candidata" })).toBeEnabled();
  });

  it("seals the saved draft under its ETag and lists the new candidate", async () => {
    let sealed = false;
    const calls = world({
      ...sealWith(() => ((sealed = true), { status: 201, body: { ...seal, published: false } })),
      [`GET ${CANDIDATES}`]: () => (sealed ? { body: { items: [seal] } } : { status: 404 }),
    });
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Selar candidata" }));
    expect(await screen.findByRole("button", { name: "Abrir v2abc" })).toBeInTheDocument();
    expect(calls.find((c) => c.path === SEALS)?.headers.get("If-Match")).toBe('"e1"');
  });

  it("says nothing changed since the published version, and what to do to seal one", async () => {
    world(sealWith(() => ({ body: { version: "v1", published: true, company: "acme", area: "cx" } })));
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Selar candidata" }));
    expect(await screen.findByText(
      "Nada mudou desde a versão publicada v1, então não há candidata a selar. Altere o rascunho e salve para selar uma.",
    )).toBeInTheDocument();
  });

  it("says what to do when the version id names other bytes", async () => {
    world(sealWith(() => ({ status: 409, body: { type: "fuseone:version-collision", title: "Version id already taken", status: 409 } })));
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Selar candidata" }));
    expect(await screen.findByText(/já identifica outro texto/)).toBeInTheDocument();
  });

  it("says which event cycle the sealed version would close", async () => {
    const eventCycleWarning = { cycles: [["triagem", "revisor"]], hopLimit: 8, truncated: false };
    world(sealWith(() => ({ status: 201, body: { ...seal, published: false, eventCycleWarning } })));
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Selar candidata" }));
    expect(await screen.findByText(/triagem → revisor/)).toBeInTheDocument();
  });

  it("says why a list could not be read, and reads it again when asked", async () => {
    let tries = 0;
    world({ [`GET ${CANDIDATES}`]: () => (++tries === 1 ? { status: 500 } : { body: { items: [seal] } }) });
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Tentar de novo" }));
    expect(await screen.findByRole("button", { name: "Abrir v2abc" })).toBeInTheDocument();
  });

  it("shows a placeholder shaped like the list while it is read", async () => {
    world({ [`GET ${CANDIDATES}`]: () => "hang" });
    await openCandidates();
    expect(await screen.findByLabelText("Lendo as candidatas")).toBeInTheDocument();
  });

  it("lists a version sealed twice once, under its newest seal and how many there are", async () => {
    const newer = { ...seal, sealId: "s-3", sealedBy: "usr_bia", sealedAt: "2026-11-20T09:00:00Z" };
    const other = { ...seal, version: "v1def", sealId: "s-2", sealedBy: "usr_caio", sealedAt: "2026-11-19T15:00:00Z" };
    world({ [`GET ${CANDIDATES}`]: () => ({ body: { items: [newer, other, seal] } }) });
    await openCandidates();
    const row = (await screen.findByRole("button", { name: "Abrir v2abc" })).closest("tr")!;
    expect(within(row as HTMLElement).getByText("usr_bia")).toBeInTheDocument();
    expect(within(row as HTMLElement).getByText("2 selos")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /^Abrir v/ })).toHaveLength(2);
    expect(screen.queryByText("usr_ana")).toBeNull();
  });

  it("opens a candidate on its sealed text, read from the candidate", async () => {
    const definition = "---\nid: triagem\n---\n\nNunca\nPrometer prazo.";
    world({
      [`GET ${CANDIDATES}`]: () => ({ body: { items: [seal] } }),
      [`GET ${CANDIDATES}/v2abc`]: () => ({ body: { ...seal, definition } }),
    });
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Abrir v2abc" }));
    const sheet = await screen.findByRole("dialog");
    expect(await within(sheet).findByText(/Prometer prazo\./)).toBeInTheDocument();
  });

  it("links a candidate to an experiment on its seal", async () => {
    world({
      [`GET ${CANDIDATES}`]: () => ({ body: { items: [seal] } }),
      [`GET ${CANDIDATES}/v2abc`]: () => ({ body: { ...seal, definition: "---\nid: triagem\n---\n" } }),
    });
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Abrir v2abc" }));
    const link = await within(await screen.findByRole("dialog")).findByRole("link", { name: "Experimentar" });
    expect(link).toHaveAttribute("href", "/agents/triagem/experiments?new=1&seal=s-1");
  });

  it("says when a candidate's sealed text can no longer be read", async () => {
    world({
      [`GET ${CANDIDATES}`]: () => ({ body: { items: [seal] } }),
      [`GET ${CANDIDATES}/v2abc`]: () => ({ status: 409, body: { type: "fuseone:candidate-unreadable", title: "unreadable", status: 409 } }),
    });
    await openCandidates();
    await userEvent.click(await screen.findByRole("button", { name: "Abrir v2abc" }));
    expect(await within(await screen.findByRole("dialog")).findByText(/não pode mais ser lido/)).toBeInTheDocument();
  });

  it("waits for unsaved edits before sealing", async () => {
    world();
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: "Editar identidade e modelo" }));
    await userEvent.type(within(await screen.findByRole("dialog")).getByLabelText(/^Nome/), " 2");
    await userEvent.keyboard("{Escape}");
    await userEvent.click(await screen.findByRole("tab", { name: "Candidatas" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Selar candidata" })).toBeDisabled());
  });
});
