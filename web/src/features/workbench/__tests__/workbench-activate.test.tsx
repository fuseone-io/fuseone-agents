import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { setLocale } from "@/i18n";
import { draftOf, openAt, polyfillPointer, serve, type Reply, type Routes } from "@/features/workbench/__tests__/workbench-fixtures";

/*
Activating a candidate from the workbench (NT-018 phase 2, release 2d). The person reads the
whole sealed text before it goes live, and is told it is then kept as every published
version is. The activation names the agent as the dialog read it: its ETag, or that it has
no version yet; anything else that moved it since is refused, and each refusal says why.
*/

const DRAFT = "/agents/triagem/draft";
const CANDIDATES = "/agents/triagem/candidates";
const ACTIVATE = `${CANDIDATES}/v2abc/activate`;
const seal = { version: "v2abc", sealId: "s-1", company: "acme", area: "cx", sealedBy: "usr_ana", sealedAt: "2026-11-19T12:00:00Z" };
const definition = "---\nid: triagem\n---\n\nNunca\nPrometer prazo.";
const live = { agent: { agentId: "triagem", versionId: "v1live", scope: { company: "acme", area: "cx" } }, versions: [] };

function world(activate: () => Reply, over: Routes = {}) {
  return serve({
    [`GET ${DRAFT}`]: () => ({ body: draftOf(), etag: '"e1"' }),
    [`GET ${DRAFT}/turns`]: () => ({ body: { items: [], conversation: "open" } }),
    [`GET ${CANDIDATES}`]: () => ({ body: { items: [seal] } }),
    [`GET ${CANDIDATES}/v2abc`]: () => ({ body: { ...seal, definition } }),
    ["GET /agents/triagem"]: () => ({ body: live, etag: '"a7"' }),
    [`POST ${ACTIVATE}`]: activate,
    ...over,
  });
}

async function openActivation() {
  openAt("/agents/triagem/workbench");
  await userEvent.click(await screen.findByRole("tab", { name: "Candidatas" }));
  await userEvent.click(await screen.findByRole("button", { name: "Abrir v2abc" }));
  await userEvent.click(await within(await screen.findByRole("dialog")).findByRole("button", { name: "Ativar" }));
  return screen.findByRole("alertdialog");
}

const confirm = async (dialog: HTMLElement) => userEvent.click(within(dialog).getByRole("button", { name: "Ativar versão" }));

describe("activating a candidate", () => {
  beforeAll(polyfillPointer);
  beforeEach(() => setLocale("pt-BR"));
  afterEach(() => vi.unstubAllGlobals());

  it("shows the whole sealed text, what it replaces, and that it is kept", async () => {
    world(() => ({ status: 500 }));
    const dialog = await openActivation();
    expect(await within(dialog).findByText(/Prometer prazo\./)).toBeInTheDocument();
    expect(within(dialog).getByText(/no lugar de v1live/)).toBeInTheDocument();
    expect(within(dialog).getByText(/guardado como toda versão publicada/)).toBeInTheDocument();
  });

  it("activates under the agent's ETag as the dialog read it", async () => {
    const calls = world(() => ({ body: { agentId: "triagem", versionId: "v2abc", created: true, sealId: "s-1", paused: false }, etag: '"a8"' }));
    await confirm(await openActivation());
    expect(await screen.findByText("v2abc está em produção.")).toBeInTheDocument();
    const sent = calls.find((c) => c.path === ACTIVATE)?.headers;
    expect(sent?.get("If-Match")).toBe('"a7"');
    expect(sent?.get("If-None-Match")).toBeNull();
  });

  it("activates an agent's first version as one, born paused", async () => {
    const calls = world(() => ({ body: { agentId: "triagem", versionId: "v2abc", created: true, sealId: "s-1", paused: true }, etag: '"a1"' }), {
      ["GET /agents/triagem"]: () => ({ status: 404 }),
    });
    const dialog = await openActivation();
    expect(await within(dialog).findByText(/primeira versão do agente/)).toBeInTheDocument();
    await confirm(dialog);
    expect(await screen.findByText("v2abc está ativa, pausada.")).toBeInTheDocument();
    expect(calls.find((c) => c.path === ACTIVATE)?.headers.get("If-None-Match")).toBe("*");
  });

  it("says which event cycle the activated version closes", async () => {
    const eventCycleWarning = { cycles: [["triagem", "revisor"]], hopLimit: 8, truncated: false };
    world(() => ({ body: { agentId: "triagem", versionId: "v2abc", created: true, paused: false, eventCycleWarning }, etag: '"a8"' }));
    await confirm(await openActivation());
    expect(await screen.findByText(/triagem → revisor/)).toBeInTheDocument();
  });

  it.each([
    ["fuseone:publish-needs-never", 400, /nunca deve fazer/],
    ["fuseone:candidate-scope-moved", 409, /mudou de escopo/],
    ["fuseone:stage-refused", 409, /autônomo/],
    ["fuseone:precondition-failed", 412, /alterou isto depois da leitura/],
  ])("says what to do when refused with %s", async (type, status, said) => {
    world(() => ({ status, body: { type, title: "refused", status } }));
    await confirm(await openActivation());
    expect(await screen.findByText(said)).toBeInTheDocument();
  });
});
