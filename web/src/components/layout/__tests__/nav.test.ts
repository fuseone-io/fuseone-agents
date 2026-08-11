import { describe, expect, it } from "vitest";
import { NAV, PAGE_TITLES } from "@/components/layout/nav";

describe("the navigation", () => {
  it("has a title for every route it links to", () => {
    // The breadcrumb falls back to the URL segment, so a missing title puts
    // "overview" at the top of a page called Visão geral.
    for (const group of NAV) {
      for (const item of group.items) {
        const segment = item.to.replace("/", "");
        expect(PAGE_TITLES[segment], `no title for ${item.to}`).toBeDefined();
      }
    }
  });
});
