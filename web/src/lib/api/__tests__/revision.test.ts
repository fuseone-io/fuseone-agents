import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { ifMatch, isStale, revised } from "@/lib/api/revision";

const read = (etag: string | null) => ({
  data: { name: "a" },
  response: new Response(null, {
    status: 200,
    headers: etag ? { ETag: etag } : {},
  }),
});

describe("a revision", () => {
  it("comes with what was read", () => {
    expect(revised(read('"v1-abc"'))).toEqual({
      body: { name: "a" },
      etag: '"v1-abc"',
    });
  });

  it("is answered by a write over it, and a first write answers none", () => {
    expect(ifMatch('"v1-abc"')).toEqual({ "If-Match": '"v1-abc"' });
    expect(ifMatch(undefined)).toEqual({});
    expect(ifMatch("")).toEqual({});
  });

  it("went stale only when the server says so", () => {
    expect(isStale(new ApiError(412))).toBe(true);
    expect(isStale(new ApiError(409))).toBe(false);
    expect(isStale(new Error("network"))).toBe(false);
  });
});

describe("a read with no revision", () => {
  // Read as no revision, the next write would send no If-Match and quietly go
  // back to last-write-wins. A resource that exists always has one.
  it("is refused rather than read as nothing to answer", () => {
    expect(() => revised(read(null))).toThrow();
  });
});
