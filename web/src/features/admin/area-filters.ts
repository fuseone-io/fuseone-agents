import type { TFunction } from "i18next";
import type { RegisteredScope } from "@/features/scope/api";

export function matchesArea(area: RegisteredScope, query: string, t: TFunction) {
  const shown = area.label || area.area;
  const haystack = [
    shown,
    area.area,
    area.company,
    `${area.company}/${area.area}`,
    t("scope.wholeCompany", { company: area.company }),
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(query);
}

export function companyOptionsFor({
  grants,
  companies,
  areas,
}: {
  grants: Array<{ company: string }>;
  companies: Array<{ id: string }>;
  areas: RegisteredScope[];
}) {
  const candidates = [
    ...companies.map((company) => company.id),
    ...grants.map((grant) => grant.company).filter((company) => company !== "*"),
    ...areas.map((area) => area.company),
  ];
  return [...new Set(candidates)].sort();
}
