import { describe, expect, it } from "vitest";
import { scopeCompanies } from "@/features/scope/scope-options";
import type { Company } from "@/features/companies/api";
import type { RegisteredScope } from "@/features/scope/api";
import type { MeGrant } from "@/features/session/api";

const grant = (company: string, area = ""): MeGrant => ({
  company,
  area,
  role: "author",
});

const company = (id: string): Company => ({
  id,
  label: id,
  areas: 0,
  archived: false,
});

const area = (company: string, name: string): RegisteredScope => ({
  company,
  area: name,
  label: name,
});

describe("scope switcher company choices", () => {
  it("expands the installation grant to real companies instead of listing the wildcard", () => {
    expect(
      scopeCompanies({
        grants: [grant("*")],
        companies: [company("acme"), company("globex")],
        areas: [area("acme", "cx")],
      }),
    ).toEqual(["acme", "globex"]);
  });

  it("keeps ordinary grant companies and visible area companies for non-global callers", () => {
    expect(
      scopeCompanies({
        grants: [grant("acme", "cx")],
        companies: [company("hidden")],
        areas: [area("globex", "support")],
      }),
    ).toEqual(["acme", "globex"]);
  });
});
