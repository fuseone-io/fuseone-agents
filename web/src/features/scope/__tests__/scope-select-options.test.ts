import { describe, expect, it } from "vitest";
import type { Company } from "@/features/companies/api";
import type { RegisteredScope } from "@/features/scope/api";
import {
  areaOptions,
  companyOptions,
  WHOLE_COMPANY,
} from "@/features/scope/scope-select-options";

const company = (id: string, archived = false): Company => ({
  id,
  label: id.toUpperCase(),
  areas: 1,
  archived,
});
const companies = [company("acme"), company("globex", true)];
const scopes: RegisteredScope[] = [
  { company: "acme", area: "finance", label: "Finance" },
  { company: "globex", area: "ops" },
];

describe("the companies a grant may name", () => {
  it("leaves a withdrawn company out of a new grant", () => {
    expect(companyOptions(companies, "acme").map((o) => o.value)).toEqual([
      "acme",
    ]);
  });

  it("keeps a withdrawn company a grant already names, and says so", () => {
    expect(companyOptions(companies, "globex")).toContainEqual({
      value: "globex",
      label: "GLOBEX",
      state: "withdrawn",
    });
  });

  it("keeps a company nobody registered, so the grant saves unchanged", () => {
    expect(companyOptions([], "legacy")).toEqual([
      { value: "legacy", label: "legacy", state: "unknown" },
    ]);
  });

  it("claims nothing about a company before the list has answered", () => {
    expect(companyOptions(undefined, "acme")).toEqual([
      { value: "acme", label: "acme", state: "active" },
    ]);
  });

  // A ceiling may apply to the installation itself, which no company names.
  it("offers the installation first when asked, and still keeps an unknown company", () => {
    expect(companyOptions(companies, "legacy", { installation: true })).toEqual([
      { value: "*", label: "", state: "installation" },
      { value: "acme", label: "ACME", state: "active" },
      { value: "legacy", label: "legacy", state: "unknown" },
    ]);
  });
});

describe("the areas a grant may name", () => {
  it("offers the whole company and the chosen company's areas only", () => {
    expect(areaOptions(scopes, "acme", "")).toEqual([
      { value: WHOLE_COMPANY, label: "", state: "whole" },
      { value: "finance", label: "Finance", state: "active" },
    ]);
  });

  it("keeps an area nobody registered", () => {
    expect(areaOptions([], "acme", "old-area")).toContainEqual({
      value: "old-area",
      label: "old-area",
      state: "unknown",
    });
  });
});
