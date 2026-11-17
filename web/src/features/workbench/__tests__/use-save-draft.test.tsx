import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSaveDraft } from "@/features/workbench/use-save-draft";
import { draftOf, serve } from "@/features/workbench/__tests__/workbench-fixtures";

/*
One save, several operations: each goes under the ETag the one before it
answered, so the draft is always named as it now is. And a refusal stops the
rest: nothing after it is sent against a draft that moved.
*/

const OPS = "/agents/triagem/draft/operations";

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>;
}

describe("saving a draft", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("names each operation's draft by the ETag the previous one answered", async () => {
    let n = 1;
    const calls = serve({ [`POST ${OPS}`]: () => ({ body: draftOf(), etag: `"e${++n}"` }) });
    const { result } = renderHook(() => useSaveDraft("triagem"), { wrapper });
    const steps = [{ kind: "add_tool", tool: "crm.reply" }, { kind: "set_reply_locale", replyLocale: "en-US" }] as const;
    await act(() => result.current.save({ body: draftOf(), etag: '"e1"' }, [...steps], { draft: () => {}, error: () => {} }));
    expect(calls.map((c) => c.headers.get("If-Match"))).toEqual(['"e1"', '"e2"']);
  });

  it("stops at a 412 and sends nothing after it", async () => {
    const calls = serve({ [`POST ${OPS}`]: () => ({ status: 412, etag: '"e9"' }) });
    const { result } = renderHook(() => useSaveDraft("triagem"), { wrapper });
    const steps = [{ kind: "add_tool", tool: "crm.reply" }, { kind: "set_reply_locale", replyLocale: "en-US" }] as const;
    await act(() => result.current.save({ body: draftOf(), etag: '"e1"' }, [...steps], { draft: () => {}, error: () => {} }));
    expect(calls).toHaveLength(1);
    expect(result.current.state).toBe("stale");
  });
});
