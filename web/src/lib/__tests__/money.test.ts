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

  it("shows stored micros as the amount a person would type", () => {
    expect(textFromMicros(1_000_000_000)).toBe("1000");
    expect(textFromMicros(250_000)).toBe("0.25");
    expect(textFromMicros(0)).toBe("");
    expect(textFromMicros(undefined)).toBe("");
  });
});
