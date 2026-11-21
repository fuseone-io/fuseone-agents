import { describe, expect, it } from "vitest";
import { fitsIn } from "@/features/workbench/rehearsal-text";

/*
The server bounds typed text in UTF-8 bytes, not in characters: a case written in
Portuguese takes more bytes than letters, and the console must refuse it where the server
would, not later.
*/
describe("a bound on typed text", () => {
  it("counts the bytes UTF-8 takes, so an accented text reaches it sooner", () => {
    expect(fitsIn(4)("ação")).toBe(false);
    expect(fitsIn(6)("ação")).toBe(true);
  });

  it("takes a text of exactly the bound and refuses one byte more", () => {
    expect(fitsIn(32 * 1024)("a".repeat(32 * 1024))).toBe(true);
    expect(fitsIn(32 * 1024)("a".repeat(32 * 1024 + 1))).toBe(false);
  });
});
