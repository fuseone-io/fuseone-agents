import { describe, expect, it } from "vitest";
import { microsFromText, textFromMicros } from "@/lib/money";

describe("money typed by a person, stored as micros", () => {
  it.each([
    ["500", 500_000_000],
    ["0,25", 250_000],
    ["1.5", 1_500_000],
    [" 12,345678 ", 12_345_678],
    ["", 0],
  ])("reads %j as %i micros", (text, micros) => {
    expect(microsFromText(text)).toBe(micros);
  });

  it.each(["abc", "-1", "1,2,3", "1e3"])(
    "refuses %j rather than guessing",
    (text) => {
      expect(microsFromText(text)).toBeNull();
    },
  );

  it.each([
    ["past the largest exact integer", "9007199254.740992"],
    ["hundreds of digits long", "9".repeat(400)],
    ["finer than a micro", "1.1234567"],
  ])("refuses an amount %s, which no integer of micros holds", (_, text) => {
    expect(microsFromText(text)).toBeNull();
  });

  it("keeps the largest exact amount to the micro", () => {
    expect(microsFromText("9007199254.740991")).toBe(Number.MAX_SAFE_INTEGER);
  });

  it.each([Number.MAX_SAFE_INTEGER, 1, 999_999, 1_000_001, 123_456_789])(
    "shows %i micros as text that reads back to the same micros",
    (micros) => {
      expect(microsFromText(textFromMicros(micros))).toBe(micros);
    },
  );

  it("shows the largest exact amount to the micro", () => {
    expect(textFromMicros(Number.MAX_SAFE_INTEGER)).toBe("9007199254.740991");
  });

  it("shows stored micros as the amount a person would type", () => {
    expect(textFromMicros(1_000_000_000)).toBe("1000");
    expect(textFromMicros(250_000)).toBe("0.25");
    expect(textFromMicros(0)).toBe("");
    expect(textFromMicros(undefined)).toBe("");
  });
});
