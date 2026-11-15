import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MoveServerDialog } from "@/features/integrations/mcp/move-server-dialog";
import { moveOffered } from "@/features/integrations/mcp/move-offer";
import { pathOf, scopeListsRoute } from "@/test/scope-lists-routes";

// Moving a server of the installation to the area that owns it (NT-019).

type Sent = { ifMatch: string | null; body: Record<string, unknown> };

beforeEach(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

function mount(answers: Response[] = []) {
  const sent: Sent[] = [];
  const onMoved = vi.fn();
  vi.stubGlobal("fetch", async (request: Request) => {
    const listed = scopeListsRoute(request.method, pathOf(request));
    if (listed) return listed;
    sent.push({ ifMatch: request.headers.get("If-Match"), body: await request.json() });
    return answers.shift() ?? new Response(null, { status: 204, headers: { ETag: '"v1-moved"' } });
  });
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MoveServerDialog name="wiki" etag={'"v1-read"'} onMoved={onMoved} onStale={vi.fn()} />
    </QueryClientProvider>,
  );
  return { sent, onMoved };
}

const problem = (status: number, body: object) =>
  new Response(JSON.stringify({ status, title: "refused", ...body }), {
    status, headers: { "Content-Type": "application/problem+json" },
  });

async function chooseArea(area: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Mover para…" }));
  await waitFor(() => expect(screen.getByRole("combobox", { name: "Empresa" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "Empresa" }));
  await user.click(screen.getByRole("option", { name: "Acme" }));
  await user.click(screen.getByRole("combobox", { name: "Área" }));
  await user.click(screen.getByRole("option", { name: area }));
  return user;
}

describe("moving a server out of the installation", () => {
  it("moves it to the chosen area, against the revision the page read", async () => {
    const { sent, onMoved } = mount();
    const user = await chooseArea("Financeiro");
    await user.click(screen.getByRole("button", { name: "Mover" }));

    await waitFor(() => expect(onMoved).toHaveBeenCalled());
    expect(sent).toEqual([{ ifMatch: '"v1-read"', body: { company: "acme", area: "finance" } }]);
  });

  it("asks for the company before sending anything", async () => {
    const { sent } = mount();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Mover para…" }));
    await user.click(screen.getByRole("button", { name: "Mover" }));

    expect(await screen.findByText("Informe a empresa.")).toBeInTheDocument();
    expect(sent).toEqual([]);
  });

  it("lists the agents it would strand, and moves only once told to strand them", async () => {
    const stranded = problem(409, {
      type: "fuseone:move-strands-agents", agents: [{ agentId: "triage", company: "acme", area: "ops" }],
    });
    const { sent, onMoved } = mount([stranded]);
    const user = await chooseArea("Financeiro");
    await user.click(screen.getByRole("button", { name: "Mover" }));

    expect(await screen.findByText("triage")).toBeInTheDocument();
    expect(onMoved).not.toHaveBeenCalled();
    await user.click(screen.getByRole("checkbox", { name: /Mover mesmo assim/ }));
    await user.click(screen.getByRole("button", { name: "Mover" }));
    await waitFor(() => expect(onMoved).toHaveBeenCalled());
    expect(sent[1]?.body).toEqual({ company: "acme", area: "finance", strand: true });
  });

  // Confirming the agents one target strands says nothing about another's.
  it("forgets the confirmation to strand when the target changes", async () => {
    const stranded = problem(409, {
      type: "fuseone:move-strands-agents", agents: [{ agentId: "triage", company: "acme", area: "ops" }],
    });
    const { sent } = mount([stranded]);
    const user = await chooseArea("Financeiro");
    await user.click(screen.getByRole("button", { name: "Mover" }));
    await user.click(await screen.findByRole("checkbox", { name: /Mover mesmo assim/ }));
    await user.click(screen.getByRole("combobox", { name: "Área" }));
    await user.click(screen.getByRole("option", { name: "Operações" }));
    await user.click(screen.getByRole("button", { name: "Mover" }));

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[1]?.body).toEqual({ company: "acme", area: "ops" });
  });

  // After a 412 nothing is sent again until the page is reloaded: a retry
  // would move a configuration nobody looked at.
  it("sends nothing more after the revision went stale", async () => {
    const { sent } = mount([problem(412, { type: "fuseone:precondition-failed" })]);
    const user = await chooseArea("Financeiro");
    await user.click(screen.getByRole("button", { name: "Mover" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/Recarregue/);
    expect(screen.getByRole("button", { name: "Mover" })).toBeDisabled();
    expect(sent).toHaveLength(1);
  });

  it("names the origin the target's egress policy does not list", async () => {
    mount([problem(400, { type: "fuseone:egress-not-allowed", detail: "https://wiki.example.com" })]);
    const user = await chooseArea("Financeiro");
    await user.click(screen.getByRole("button", { name: "Mover" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/acme\/finance não lista https:\/\/wiki\.example\.com/);
  });
});

describe("when the page offers the move", () => {
  it("offers it to the platform for a remote server at the installation, and in no other case", () => {
    const platform = { platformConfigure: true };
    expect(moveOffered(platform, { transport: "http" })).toBe(true);
    expect(moveOffered(platform, { transport: "http", scopeKind: "installation" })).toBe(true);
    expect(moveOffered(platform, { transport: "stdio" })).toBe(false);
    expect(moveOffered(platform, { transport: "http", scopeKind: "area" })).toBe(false);
    expect(moveOffered({ platformConfigure: false }, { transport: "http" })).toBe(false);
  });
});
