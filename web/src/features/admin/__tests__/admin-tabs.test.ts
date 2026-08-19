import { describe, expect, it } from "vitest";
import { visibleAdminTabs } from "@/features/admin/admin-tabs";

describe("administration tabs", () => {
  it("does not treat tool reading as access to administration", () => {
    expect(visibleAdminTabs(["tool:read"]).map((tab) => tab.value)).toEqual([]);
  });

  it("shows only the sections whose permission the caller holds", () => {
    expect(visibleAdminTabs(["audit:read"]).map((tab) => tab.value)).toEqual([
      "events",
    ]);
    expect(
      visibleAdminTabs(["identity:write"]).map((tab) => tab.value),
    ).toEqual(["identity", "people"]);
  });

  it("keeps an unknown session different from an open installation", () => {
    expect(visibleAdminTabs(undefined).map((tab) => tab.value)).toEqual([]);
    expect(visibleAdminTabs(null).map((tab) => tab.value)).toEqual([
      "tools",
      "branding",
      "authoring",
      "companies",
      "areas",
      "identity",
      "people",
      "prices",
      "budgets",
      "retention",
      "events",
    ]);
  });
});
