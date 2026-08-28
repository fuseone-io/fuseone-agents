import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RememberThisButton } from "@/features/runs/remember-this-button";
import { sessionKeys } from "@/features/session/api";
import type { Step } from "@/lib/api/client";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const FINISHED: Step = {
  seq: 3,
  kind: "run_finished",
  at: "2026-08-18T12:00:00Z",
  hash: "h",
  labels: ["channel:email"],
  payload: {
    outcome_ref: "run://run-1/3/abc",
    outcome_digest: "ab".repeat(32),
  },
};

const TRAIL = {
  items: [
    { seq: 1, kind: "run_started", at: "t", hash: "h", labels: ["untrusted"] },
    FINISHED,
  ],
};

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * Every request the sheet makes, answered at the network boundary. Posted
 * bodies are collected so a test can assert what the console actually sent.
 *
 * The session is not among them: it is seeded into the cache below, and asking
 * for it over the network fails here rather than falling back. Answering it in
 * both places would let the permission tests pass on whichever source the
 * component did not use.
 */
function stubNetwork(posted: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      // By path, not by substring. "/admin/memory/assertions" contains "/me",
      // so a substring match answered the memory being written with the
      // session — and the test read the missing write as a form that never
      // submitted.
      const path = new URL(url, "http://localhost").pathname;
      if (path.endsWith("/steps")) return json(TRAIL);
      if (path.endsWith("/runs/run-1")) {
        return json({ runId: "run-1", scope: { company: "acme", area: "ops" } });
      }
      if (path.endsWith("/memory/assertions")) {
        // The client builds a Request, so the body is on it rather than in
        // init. Reading only init left every assertion about what was sent
        // looking at an empty object, which agrees with anything.
        const body =
          input instanceof Request
            ? await input.clone().text()
            : String(init?.body ?? "");
        posted.push(JSON.parse(body));
        return json({ id: "m1" });
      }
      throw new Error(`unexpected request: ${path}`);
    }),
  );
}

/**
 * The session is seeded rather than awaited. In the product SessionGate has
 * already resolved it before any run screen renders, so a test that lets it
 * arrive mid-render is testing a state the console does not have — and it says
 * so, by updating outside act.
 */
function renderButton(step: Step = FINISHED, can = ["agent:publish"]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(sessionKeys.me, { id: "u", can });
  return render(
    <QueryClientProvider client={client}>
      <RememberThisButton runId="run-1" step={step} />
    </QueryClientProvider>,
  );
}

async function openSheet() {
  await userEvent.click(screen.getByRole("button", { name: /lembrar disso/i }));
  return screen.findByRole("dialog");
}

async function fillTheFact(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/tipo/i), "policy");
  await user.type(screen.getByLabelText(/^assunto/i), "refunds");
  await user.type(screen.getByLabelText(/assinatura/i), "refund.limit");
  await user.type(screen.getByLabelText(/afirmação/i), "limit is R$ 500");
  await user.type(screen.getByLabelText(/por quê/i), "reviewed after the call");
}

describe("teaching a memory from a run", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("sends the run and the artifact, and never the digest or the labels", async () => {
    const posted: unknown[] = [];
    stubNetwork(posted);
    const user = userEvent.setup();
    renderButton();

    await openSheet();
    await fillTheFact(user);
    await user.click(screen.getByRole("button", { name: /salvar memória/i }));

    await vi.waitFor(() => expect(posted).toHaveLength(1));
    // The citation names a run and one of its outputs. The digest, the labels
    // and the agent are the ledger's answers and are not sent — a body that
    // carried them would be a body asserting its own provenance.
    expect(posted[0]).toEqual({
      company: "acme",
      area: "ops",
      namespace: "agent",
      kind: "policy",
      subject: "refunds",
      signature: "refund.limit",
      claim: "limit is R$ 500",
      evidence: [{ runId: "run-1", artifact: "final_answer" }],
      reason: "reviewed after the call",
    });
  });

  // The taint of the run reaches the memory, so it is on screen before the
  // decision rather than in the record after it.
  it("shows the labels the run had accumulated by the cited step", async () => {
    stubNetwork([]);
    renderButton();
    const sheet = await openSheet();

    expect(await within(sheet).findByText("untrusted")).toBeInTheDocument();
    expect(within(sheet).getByText("channel:email")).toBeInTheDocument();
  });

  // Every part of it is the ledger's answer, so there is nothing here to change
  // — and a field somebody can change is one they can change to something the
  // run never produced.
  it("shows the citation without offering to edit it", async () => {
    stubNetwork([]);
    renderButton();
    const sheet = await openSheet();
    const evidence = within(sheet).getByRole("region", { name: /evidência/i });

    expect(within(evidence).getByText(/run-1 · #3 · final_answer/)).toBeInTheDocument();
    expect(within(evidence).queryAllByRole("textbox")).toHaveLength(0);
  });

  // Picking among the names the ledger recorded. The chip has to follow the
  // choice, and the request has to carry it — a selector the body ignores would
  // record a memory citing the closing answer while the screen said otherwise.
  it("cites the output that was chosen, not the one offered first", async () => {
    const posted: unknown[] = [];
    stubNetwork(posted);
    const user = userEvent.setup();
    renderButton({
      ...FINISHED,
      payload: {
        ...FINISHED.payload,
        artifacts: [
          { name: "triage", ref: "run://run-1/3/cd", digest: "cd".repeat(32) },
        ],
      },
    });

    const sheet = await openSheet();
    await user.click(within(sheet).getByRole("radio", { name: "triage" }));

    expect(
      within(sheet).getByText(/run-1 · #3 · triage · cdcdcdcd/),
    ).toBeInTheDocument();

    await fillTheFact(user);
    await user.click(screen.getByRole("button", { name: /salvar memória/i }));

    await vi.waitFor(() => expect(posted).toHaveLength(1));
    expect((posted[0] as { evidence: unknown }).evidence).toEqual([
      { runId: "run-1", artifact: "triage" },
    ]);
  });

  // Shared memory is what every agent in the scope recalls, so it has to be
  // reachable as well as never the default.
  it("sends the shared namespace when somebody chooses it", async () => {
    const posted: unknown[] = [];
    stubNetwork(posted);
    const user = userEvent.setup();
    renderButton();

    const sheet = await openSheet();
    await user.click(within(sheet).getByRole("radio", { name: /todos/i }));
    await fillTheFact(user);
    await user.click(screen.getByRole("button", { name: /salvar memória/i }));

    await vi.waitFor(() => expect(posted).toHaveLength(1));
    expect((posted[0] as { namespace: string }).namespace).toBe("shared");
  });

  // A trail that has not reached the cited step folds to fewer labels, not to
  // none — so saying "no labels" here would understate the taint of the thing
  // being taught, at the moment somebody decides to teach it.
  it("says it is still reading rather than that there are no labels", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | RequestInfo | URL) => {
        const url = input instanceof Request ? input.url : String(input);
        const path = new URL(url, "http://localhost").pathname;
        // A page that does not start at the run's opening step: the console
        // holds part of the trail and cannot yet fold it.
        if (path.endsWith("/steps")) return json({ items: [FINISHED] });
        return json({ runId: "run-1", scope: { company: "acme", area: "ops" } });
      }),
    );
    renderButton();
    const sheet = await openSheet();

    expect(
      await within(sheet).findByLabelText(/lendo a trilha/i),
    ).toBeInTheDocument();
    expect(within(sheet).queryByText(/sem labels/i)).not.toBeInTheDocument();
  });

  it("is not offered to somebody who cannot publish", () => {
    stubNetwork([]);
    renderButton(FINISHED, ["agent:read"]);

    expect(
      screen.queryByRole("button", { name: /lembrar disso/i }),
    ).not.toBeInTheDocument();
  });

  it("is not offered on a step no memory can cite", async () => {
    stubNetwork([]);
    renderButton({
      seq: 2,
      kind: "tool_returned",
      at: "t",
      hash: "h",
      payload: { result_ref: "run://run-1/2/ab" },
    });

    expect(
      screen.queryByRole("button", { name: /lembrar disso/i }),
    ).not.toBeInTheDocument();
  });
});
