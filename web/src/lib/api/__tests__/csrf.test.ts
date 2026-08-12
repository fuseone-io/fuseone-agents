import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "@/lib/api/client";

function captureRequests() {
  const seen: Request[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    seen.push(request);
    return new Response("{}", {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  return seen;
}

afterEach(() => {
  vi.unstubAllGlobals();
  document.cookie = "fuseone_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT";
});

describe("the API client's CSRF double submit", () => {
  it("echoes the cookie on a write, so the server can tell the console apart from someone else's form", async () => {
    document.cookie = "fuseone_csrf=abc123";
    const seen = captureRequests();

    await api.POST("/runs/{runId}/approvals", {
      params: { path: { runId: "run_1" } },
      body: { atSeq: 1, approved: true },
    });

    expect(seen[0]?.headers.get("X-CSRF-Token")).toBe("abc123");
  });

  it("leaves a read alone: a header on a GET would be noise the server never checks", async () => {
    document.cookie = "fuseone_csrf=abc123";
    const seen = captureRequests();

    await api.GET("/approvals", { params: { query: {} } });

    expect(seen[0]?.headers.get("X-CSRF-Token")).toBeNull();
  });

  it("sends the write without a token rather than inventing one when no cookie exists", async () => {
    const seen = captureRequests();

    await api.POST("/runs/{runId}/approvals", {
      params: { path: { runId: "run_1" } },
      body: { atSeq: 1, approved: true },
    });

    expect(seen[0]?.headers.get("X-CSRF-Token")).toBeNull();
  });
});
