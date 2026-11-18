import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { WorkbenchNewPage } from "@/features/workbench/workbench-new-page";
import { WorkbenchPage } from "@/features/workbench/workbench-page";
import type { AgentDraft } from "@/lib/api/client";

/*
The network boundary for the workbench's tests: every request is recorded with
its method, path, headers and body, and answered by the route a test gives,
or by an empty list for any other read. Nothing of the panel is mocked.
*/

export type Call = { method: string; path: string; headers: Headers; body: unknown };
export type Reply = { status?: number; body?: unknown; etag?: string } | "hang";
export type Routes = Record<string, (call: Call, calls: Call[]) => Reply | Promise<Reply>>;

export function serve(routes: Routes): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      const text = await request.text();
      const call: Call = {
        method: request.method,
        path: url.pathname.replace("/api/v1", ""),
        headers: request.headers,
        body: text ? JSON.parse(text) : undefined,
      };
      calls.push(call);
      const route = routes[`${call.method} ${call.path}`];
      const reply = route ? await route(call, calls) : call.method === "GET" ? { body: { items: [] } } : { status: 404 };
      if (reply === "hang") return new Promise<Response>(() => {});
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (reply.etag) headers.ETag = reply.etag;
      const status = reply.status ?? 200;
      const payload = reply.body ?? (status >= 400 ? { title: "refused", status } : {});
      return new Response(status === 204 ? null : JSON.stringify(payload), { status, headers });
    }),
  );
  return calls;
}

export function openAt(path: string, around?: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        {around}
        <Routes>
          <Route path="/agents/workbench" element={<WorkbenchNewPage />} />
          <Route path="/agents/:agentId/workbench" element={<WorkbenchPage />} />
        </Routes>
      </MemoryRouter>
      <Toaster />
    </QueryClientProvider>,
  );
}

export function draftOf(over: Partial<AgentDraft["definition"]> = {}, base = "v1"): AgentDraft {
  return {
    agentId: "triagem",
    draftId: "d-1",
    baseVersionId: base || undefined,
    definition: {
      name: "Triagem",
      company: "acme",
      area: "cx",
      provider: "openai",
      model: "m",
      instructions: "Objetivo\nTriar e responder.",
      tools: ["crm.lookup"],
      budget: { micros: 500_000, steps: 60 },
      triggers: [],
      ...over,
    },
    blocks: [{ id: "b1", kind: "objective", label: "Objetivo", text: "Triar e responder." }],
    missing: [],
  };
}

export const PUBLISHED = {
  agent: {
    agentId: "triagem",
    versionId: "v1",
    scope: { company: "acme", area: "cx" },
    name: "Triagem",
    provider: "openai",
    model: "m",
    tools: ["crm.lookup"],
    budget: { micros: 500_000, steps: 60 },
    triggers: [],
    publishedAt: "2026-11-17T12:00:00Z",
    latest: true,
    stage: "draft",
  },
  instructions: "Objetivo\nTriar.",
  versions: [],
};

export const SCOPES = { items: [{ company: "acme", area: "cx", label: "Atendimento" }] };

/** Radix reads pointer capture and scrolls into view, which jsdom lacks. */
export function polyfillPointer() {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.setPointerCapture ??= () => {};
  Element.prototype.releasePointerCapture ??= () => {};
  // As current Chrome does: scrollIntoView answers a promise, which an effect
  // must not return.
  // Assigned, not ??=: the global setup already gives one that answers nothing.
  Element.prototype.scrollIntoView = (() => Promise.resolve()) as unknown as Element["scrollIntoView"];
}
