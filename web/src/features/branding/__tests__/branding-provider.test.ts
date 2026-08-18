import { describe, expect, it } from "vitest";
import {
  brandCSSVariables,
  defaultBranding,
  normaliseBranding,
} from "@/features/branding/model";

describe("installation branding", () => {
  it("falls back to the product brand when no installation value exists", () => {
    expect(normaliseBranding(undefined)).toEqual(defaultBranding);
  });

  it("derives a readable primary palette from one configured colour", () => {
    const vars = brandCSSVariables({
      displayName: "Acme Agents",
      primaryColor: "#F6D34A",
    });

    expect(vars["--fuse-500" as keyof typeof vars]).toBe("#f6d34a");
    expect(vars["--primary-foreground" as keyof typeof vars]).toBe("#0b1416");
    expect(vars["--surface-accent" as keyof typeof vars]).toBe(
      "rgb(246 211 74 / 0.12)",
    );
  });
});
