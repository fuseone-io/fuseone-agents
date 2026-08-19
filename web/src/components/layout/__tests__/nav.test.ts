import { describe, expect, it } from "vitest";
import { NAV, PAGE_TITLES, navItemVisible } from "@/components/layout/nav";

function item(to: string) {
  for (const group of NAV) {
    const found = group.items.find((candidate) => candidate.to === to);
    if (found) return found;
  }
  throw new Error(`no nav item for ${to}`);
}

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

  it("keeps integrations available to tool readers without making them administrators", () => {
    const can = ["tool:read"];

    expect(navItemVisible(item("/integrations"), can)).toBe(true);
    expect(navItemVisible(item("/admin"), can)).toBe(false);
  });

  it("shows administration for permissions used by administration screens", () => {
    expect(navItemVisible(item("/admin"), ["identity:write"])).toBe(true);
    expect(navItemVisible(item("/admin"), ["audit:read"])).toBe(true);
  });

  it("does not read an unknown session as the open installation mode", () => {
    expect(navItemVisible(item("/admin"), undefined)).toBe(false);
    expect(navItemVisible(item("/manual"), undefined)).toBe(true);
  });

  it("keeps the deliberate open-installation mode unrestricted", () => {
    expect(navItemVisible(item("/admin"), null)).toBe(true);
  });
});
