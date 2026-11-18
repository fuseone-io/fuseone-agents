import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve, type Reply } from "@/features/workbench/__tests__/workbench-fixtures";

const OPS = "/agents/triagem/draft/operations";

/*
Publishing from the workbench (NT-018, contracts 4 and 10): the saved draft
becomes the agent's next version, under the draft's ETag, after the person
confirms. Each refusal says what to do about it.
*/

const DRAFT = "/agents/triagem/draft";
const PUBLISH = `${DRAFT}/publish`;

function world(publish: () => Reply) {
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
    [`POST ${PUBLISH}`]: publish,
  });
}

async function publishConfirmed() {
  await userEvent.click(await screen.findByRole("button", { name: "Publicar" }));
  await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Publicar versão" }));
}

describe("publishing from the workbench", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("publishes the saved draft under its ETag, once confirmed", async () => {
    const calls = world(() => ({ body: { agentId: "triagem", versionId: "v2abc", created: true, paused: true, draft: draftOf({}, "v2abc") }, etag: '"e1"' }));
    openAt("/agents/triagem/workbench");
    await publishConfirmed();
    expect(await screen.findByText("Publicado como v2abc, pausado")).toBeInTheDocument();
    expect(calls.find((c) => c.path === PUBLISH)?.headers.get("If-Match")).toBe('"e1"');
  });

  it("waits for unsaved edits to be saved first", async () => {
    world(() => ({ status: 500 }));
    openAt("/agents/triagem/workbench");
    await userEvent.click(await screen.findByRole("button", { name: "Editar identidade e modelo" }));
    await userEvent.type(within(await screen.findByRole("dialog")).getByLabelText(/^Nome/), " 2");
    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Publicar" })).toBeDisabled();
    expect(screen.getByText(/^Salve as alterações antes de publicar/)).toBeInTheDocument();
  });

  it.each([
    ["fuseone:publish-needs-never", 400, "Diga nas instruções o que o agente nunca deve fazer antes de publicar"],
    ["fuseone:agent-moved", 412, "O agente foi publicado por outro caminho depois que este rascunho foi aberto"],
  ])("says what to do when refused with %s", async (type, status, said) => {
    world(() => ({ status, body: { type, title: "refused", status } }));
    openAt("/agents/triagem/workbench");
    await publishConfirmed();
    expect(await screen.findByText(new RegExp(said))).toBeInTheDocument();
  });

  // A discard landing while the publication is on its way would leave the
  // person reading a draft that no longer exists.
  it("holds the discard while a publication is on its way", async () => {
    world(() => "hang");
    openAt("/agents/triagem/workbench");
    await publishConfirmed();
    expect(await screen.findByRole("button", { name: "Descartar rascunho" })).toBeDisabled();
  });

  // A save that lands while the publication is answering is newer than the
  // snapshot the publication answers with: it stays on screen and in the
  // cache, and only the base moves.
  it("keeps a draft saved while the publication was answering", async () => {
    let answer: (reply: Reply) => void = () => {};
    const calls = serve({
      [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
      [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
      [`POST ${PUBLISH}`]: () => new Promise<Reply>((resolve) => (answer = resolve)),
      [`POST ${OPS}`]: () => ({ body: draftOf({ name: "Triagem nova" }), etag: '"e2"' }),
    });
    openAt("/agents/triagem/workbench");
    await publishConfirmed();
    await userEvent.click(await screen.findByRole("button", { name: "Editar identidade e modelo" }));
    await userEvent.type(within(await screen.findByRole("dialog")).getByLabelText(/^Nome/), " nova");
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho/ }));
    await screen.findByText("Rascunho salvo");
    answer({ body: { agentId: "triagem", versionId: "v2abc", created: true, paused: true, draft: draftOf({}, "v2abc") }, etag: '"e1"' });
    await screen.findByText("Publicado como v2abc, pausado");
    expect(screen.getByRole("tabpanel", { name: "Rascunho" })).toHaveTextContent("Triagem nova");
    await userEvent.click(screen.getByRole("button", { name: "Editar identidade e modelo" }));
    await userEvent.type(within(await screen.findByRole("dialog")).getByLabelText(/^Nome/), "!");
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: /Salvar no rascunho/ }));
    expect(calls.filter((c) => c.path === OPS).at(-1)?.headers.get("If-Match")).toBe('"e2"');
  });
});
