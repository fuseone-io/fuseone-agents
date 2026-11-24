import { screen, waitFor, within } from "@testing-library/react";
import { Link } from "react-router-dom";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import {
  PUBLISHED,
  SCOPES,
  draftOf,
  openAt,
  polyfillPointer,
  serve,
  type Call,
} from "@/features/workbench/__tests__/workbench-fixtures";

/*
The workbench's panel, walked as a person walks it (NT-018 1b, part 4).

It works on the draft alone: every save is the draft's operations, each under
the ETag the previous answer named, and nothing here publishes. A refusal —
somebody wrote first, or the draft is gone — keeps what the person typed.
*/

const DRAFT = "/agents/triagem/draft";
const OPS = "/agents/triagem/draft/operations";
const ops = (calls: Call[]) => calls.filter((c) => c.method === "POST" && c.path === OPS);

/** Renames the agent in the identity's side panel, and closes it. */
async function rename(to: string, append = false) {
  await userEvent.click(await screen.findByRole("button", { name: /Editar identidade e modelo|Edit identity and model/ }));
  const name = within(await screen.findByRole("dialog")).getByLabelText(/^(Nome|Name)/);
  if (!append) await userEvent.clear(name);
  await userEvent.type(name, to);
  await userEvent.keyboard("{Escape}");
}

const summary = () => screen.getByRole("tabpanel", { name: /Rascunho|Draft/ });

describe("the workbench panel", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("shows the draft's shape while it loads", async () => {
    serve({ [`GET ${DRAFT}`]: () => "hang" });
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("Carregando")).toBeInTheDocument();
  });

  it("says what failed and reads again when asked", async () => {
    let reads = 0;
    // The read and the one retry every read gets both fail.
    serve({
      [`GET ${DRAFT}`]: () => (++reads <= 2 ? { status: 500 } : { body: draftOf(), etag: '"e1"' }),
    });
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: /Tentar de novo/ }, { timeout: 4000 }));
    expect(await screen.findByRole("heading", { name: "triagem" })).toBeInTheDocument();
  });

  it("opens a published agent's draft, untouched, with If-None-Match: *", async () => {
    const calls = serve({
      [`GET ${DRAFT}`]: () => ({ status: 404 }),
      "GET /agents/triagem": () => ({ body: PUBLISHED }),
      [`POST ${DRAFT}`]: () => ({ status: 201, body: draftOf(), etag: '"e1"' }),
    });
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("Este agente ainda não tem rascunho")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Abrir rascunho/ }));
    expect(await screen.findByRole("heading", { name: "triagem" })).toBeInTheDocument();
    const created = calls.find((c) => c.method === "POST" && c.path === DRAFT);
    expect(created?.headers.get("If-None-Match")).toBe("*");
    expect(created?.body).toBeUndefined();
  });

  it("points an identifier with nothing behind it to a new agent's first step", async () => {
    serve({ [`GET ${DRAFT}`]: () => ({ status: 404 }), "GET /agents/triagem": () => ({ status: 404 }) });
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("Nenhum agente ou rascunho com este identificador")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Criar agente na bancada/ })).toHaveAttribute("href", "/agents/new");
  });

  it("saves each change under the ETag the last answer named", async () => {
    const calls = serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
      [`POST ${OPS}`]: (call) => ({ body: draftOf({ name: (call.body as { name: string }).name }), etag: `"e${ops(calls).length + 1}"` }),
    });
    openAt("/agents/triagem/workbench");
    await rename("Triagem nova");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho · 1 alteração/ }));
    await screen.findByRole("button", { name: /Nada a salvar/ });
    await rename("Triagem de novo");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho/ }));
    await waitFor(() => expect(ops(calls)).toHaveLength(2));
    expect(ops(calls).map((c) => c.headers.get("If-Match"))).toEqual(['"e1"', '"e2"']);
    expect(ops(calls)[1]?.body).toMatchObject({ kind: "set_identity", name: "Triagem de novo" });
  });

  it("keeps the edit through a 412 and saves it over the draft read again", async () => {
    let reads = 0;
    const calls = serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf(reads++ ? { model: "outro" } : {}), etag: reads > 1 ? '"e2"' : '"e1"' }),
      [`POST ${OPS}`]: (call) => (call.headers.get("If-Match") === '"e1"' ? { status: 412, etag: '"e2"' } : { body: draftOf(), etag: '"e3"' }),
    });
    openAt("/agents/triagem/workbench");
    await rename("Triagem nova");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho/ }));
    expect(await screen.findByText("O rascunho mudou desde que você o leu")).toBeInTheDocument();
    expect(summary()).toHaveTextContent("Triagem nova");
    await userEvent.click(screen.getByRole("button", { name: /Reler o rascunho/ }));
    await waitFor(() => expect(screen.queryByText("O rascunho mudou desde que você o leu")).toBeNull());
    expect(summary()).toHaveTextContent("Triagem nova");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho/ }));
    await waitFor(() => expect(ops(calls).at(-1)?.headers.get("If-Match")).toBe('"e2"'));
  });

  it("keeps the edit on screen when the draft is gone, and after reading again", async () => {
    let reads = 0;
    serve({
      [`GET ${DRAFT}`]: () => (reads++ === 0 ? { body: draftOf(), etag: '"e1"' } : { status: 404 }),
      [`POST ${OPS}`]: () => ({ status: 404 }),
    });
    openAt("/agents/triagem/workbench");
    await rename("Triagem nova");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho/ }));
    expect(await screen.findByText("Este rascunho não está mais disponível")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Tentar de novo/ }));
    await waitFor(() => expect(reads).toBeGreaterThan(1));
    expect(screen.getByText("Este rascunho não está mais disponível")).toBeInTheDocument();
    expect(summary()).toHaveTextContent("Triagem nova");
  });

  // Publishing is the workbench's own door now (NT-018, contract 4); the
  // instructions still have no field of their own.
  it("reads the instructions against the published version, with no field to edit them", async () => {
    serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
      "GET /agents/triagem": () => ({ body: PUBLISHED }),
    });
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("Diferença para a versão publicada")).toBeInTheDocument();
    expect(await screen.findByText("responder", { exact: false, selector: "ins" })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: /Instruções/ })).toBeNull();
  });
});

describe("a new agent's first step", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("opens the draft with the identity a person chose, then shows it", async () => {
    const calls = serve({
      "GET /admin/scopes": () => ({ body: SCOPES }),
      [`POST ${DRAFT}`]: () => ({ status: 201, body: draftOf({}, ""), etag: '"e1"' }),
      [`GET ${DRAFT}`]: () => ({ body: draftOf({}, ""), etag: '"e1"' }),
    });
    openAt("/agents/workbench");
    await userEvent.type(await screen.findByLabelText("Identificador"), "triagem");
    await userEvent.type(screen.getByLabelText("Nome"), "Triagem");
    await userEvent.click(screen.getByRole("combobox", { name: "Área" }));
    await userEvent.click(await screen.findByRole("option", { name: /Atendimento/ }));
    await userEvent.click(screen.getByRole("button", { name: /Criar rascunho/ }));
    expect(await screen.findByRole("heading", { name: "triagem" })).toBeInTheDocument();
    const created = calls.find((c) => c.method === "POST" && c.path === DRAFT);
    expect(created?.headers.get("If-None-Match")).toBe("*");
    expect(created?.body).toEqual({ name: "Triagem", company: "acme", area: "cx" });
  });

  it("names each tool no step reaches, since a run never receives it", async () => {
    serve({
      [`GET ${DRAFT}`]: () => ({ body: { ...draftOf(), unplaced: ["rootly.create_alert"] }, etag: '"e1"' }),
    });
    openAt("/agents/triagem/workbench");
    const notice = await screen.findByRole("region", { name: /Nenhum passo alcança/ });
    expect(within(notice).getByText("rootly.create_alert")).toBeInTheDocument();
    expect(screen.queryByText("Nada falta para publicar.")).not.toBeInTheDocument();
  });

  it("asks for what is missing before it sends anything", async () => {
    const calls = serve({ "GET /admin/scopes": () => ({ body: SCOPES }) });
    openAt("/agents/workbench");
    await userEvent.click(await screen.findByRole("button", { name: /Criar rascunho/ }));
    expect(await screen.findByText("Escolha um identificador")).toBeInTheDocument();
    expect(calls.some((c) => c.method === "POST")).toBe(false);
  });
});

describe("the workbench in English", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("en-US"));
  afterEach(() => {
    vi.unstubAllGlobals();
    setLocale("pt-BR");
  });

  it("says a published agent has no draft yet, and how to open one", async () => {
    serve({ [`GET ${DRAFT}`]: () => ({ status: 404 }), "GET /agents/triagem": () => ({ body: PUBLISHED }) });
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("This agent has no draft yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Open a draft/ })).toBeInTheDocument();
  });

  it("names the refusal and what was kept when the draft changed underneath", async () => {
    serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
      [`POST ${OPS}`]: () => ({ status: 412, etag: '"e2"' }),
    });
    openAt("/agents/triagem/workbench");
    await rename(" v2", true);
    await userEvent.click(screen.getByRole("button", { name: /Save to draft · 1 change/ }));
    expect(await screen.findByText("The draft changed since you read it")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Read the draft again/ })).toBeInTheDocument();
  });

  it("shows the published difference and what is still missing, in English", async () => {
    serve({
      [`GET ${DRAFT}`]: () => ({ body: { ...draftOf(), missing: ["tools"] }, etag: '"e1"' }),
      "GET /agents/triagem": () => ({ body: PUBLISHED }),
    });
    openAt("/agents/triagem/workbench");
    expect(await screen.findByText("Difference from the published version")).toBeInTheDocument();
    expect(screen.getByText("Still needed before publishing")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Tools" })).toBeInTheDocument();
  });
});

describe("the workbench panel, between agents and during a save", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("shows another agent's own draft, not the edits left on the one before", async () => {
    serve({
      "GET /agents/a/draft": () => ({ body: { ...draftOf({ name: "Agente A" }), agentId: "a", draftId: "da" }, etag: '"a1"' }),
      "GET /agents/b/draft": () => ({ body: { ...draftOf({ name: "Agente B" }), agentId: "b", draftId: "db" }, etag: '"b1"' }),
    });
    openAt("/agents/b/workbench", <><Link to="/agents/a/workbench">ir para a</Link><Link to="/agents/b/workbench">ir para b</Link></>);
    await screen.findByRole("heading", { name: "b" });
    await userEvent.click(screen.getByRole("link", { name: "ir para a" }));
    await rename("Edição de A");
    await userEvent.click(screen.getByRole("link", { name: "ir para b" }));
    await screen.findByRole("heading", { name: "b" });
    expect(summary()).toHaveTextContent("Agente B");
    expect(summary()).not.toHaveTextContent("Edição de A");
  });

  it("keeps an edit made while a save was on its way", async () => {
    let answer: (reply: { body: unknown; etag: string }) => void = () => {};
    serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
      [`POST ${OPS}`]: () => new Promise((resolve) => (answer = resolve)),
    });
    openAt("/agents/triagem/workbench");
    await rename("Primeira");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho/ }));
    await rename("Segunda");
    answer({ body: draftOf({ name: "Primeira" }), etag: '"e2"' });
    await screen.findByRole("button", { name: /Salvar no rascunho · 1 alteração/ });
    expect(summary()).toHaveTextContent("Segunda");
  });
});
