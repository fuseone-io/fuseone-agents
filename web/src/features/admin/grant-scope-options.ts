import type { Company } from "@/features/companies/api";
import type { RegisteredScope } from "@/features/scope/api";

/**
 * What the company and area selects of a grant offer.
 *
 * A select can only show a value it lists, so whatever a grant already names
 * is always listed — a withdrawn company, or a scope from before areas were
 * registered, or one the lists failed to bring. Otherwise opening the editor
 * and saving would silently change a grant nobody touched.
 */

/** The area value of a grant over a whole company. A select item cannot be "". */
export const WHOLE_COMPANY = "__whole_company__";

export type ScopeOptionState =
  | "active"
  | "withdrawn"
  | "unknown"
  | "whole"
  | "installation";

export type ScopeOption = {
  value: string;
  label: string;
  state: ScopeOptionState;
};

const INSTALLATION = "*";

export function companyOptions(
  companies: Company[] | undefined,
  current: string,
): ScopeOption[] {
  const listed = companies ?? [];
  const offered: ScopeOption[] = listed
    .filter((c) => !c.archived || c.id === current)
    .map((c) => ({
      value: c.id,
      label: c.label || c.id,
      state: c.archived ? "withdrawn" : "active",
    }));
  if (current === INSTALLATION) {
    return [{ value: INSTALLATION, label: "", state: "installation" }, ...offered];
  }
  if (current && !listed.some((c) => c.id === current)) {
    offered.push(asHeld(current, companies));
  }
  return offered;
}

export function areaOptions(
  scopes: RegisteredScope[] | undefined,
  company: string,
  current: string,
): ScopeOption[] {
  if (company === INSTALLATION) {
    return [{ value: WHOLE_COMPANY, label: "", state: "installation" }];
  }
  const inCompany = (scopes ?? []).filter((s) => s.company === company);
  const offered: ScopeOption[] = [
    { value: WHOLE_COMPANY, label: "", state: "whole" },
    ...inCompany.map((s) => ({
      value: s.area,
      label: s.label || s.area,
      state: "active" as const,
    })),
  ];
  if (current && !inCompany.some((s) => s.area === current)) {
    offered.push(asHeld(current, scopes));
  }
  return offered;
}

// Unknown only once the list has answered: before then, or when it failed,
// nothing is known about the value either way, and saying "not registered"
// would be a claim nobody checked.
function asHeld(current: string, list: unknown[] | undefined): ScopeOption {
  return {
    value: current,
    label: current,
    state: list === undefined ? "active" : "unknown",
  };
}

/** The first company a new grant can name, or the one the setup token made. */
export function firstOffered(companies: Company[] | undefined): string {
  return companies?.find((c) => !c.archived)?.id ?? "default";
}
