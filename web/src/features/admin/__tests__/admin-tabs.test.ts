import { describe, expect, it } from "vitest";
import {
  visibleAdminTabGroups,
  visibleAdminTabs,
} from "@/features/admin/admin-tabs";

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
      "events",
      "branding",
      "authoring",
      "identity",
      "companies",
      "areas",
      "people",
      "prices",
      "budgets",
      "retention",
    ]);
  });

  it("groups sections by the administrative decision they belong to", () => {
    expect(
      visibleAdminTabGroups(["identity:write"]).map((group) => [
        group.label,
        group.tabs.map((tab) => tab.value),
      ]),
    ).toEqual([
      ["admin.group.platform", ["identity"]],
      ["admin.group.people", ["people"]],
    ]);
  });

  it("places every visible section in exactly one group", () => {
    const flat = visibleAdminTabs(null).map((tab) => tab.value);
    const grouped = visibleAdminTabGroups(null).flatMap((group) =>
      group.tabs.map((tab) => tab.value),
    );

    expect(grouped).toEqual(flat);
    expect(new Set(grouped).size).toBe(grouped.length);
  });
});
