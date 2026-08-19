import type { TFunction } from "i18next";
import type { Company } from "@/features/companies/api";

export const COMPANY_VIEWS = ["all", "active", "withdrawn"] as const;
export type CompanyView = (typeof COMPANY_VIEWS)[number];

export function matchesCompanyView(company: Company, view: CompanyView) {
  if (view === "active") return !company.archived;
  if (view === "withdrawn") return company.archived;
  return true;
}

export function matchesCompany(company: Company, query: string, t: TFunction) {
  const haystack = [
    company.id,
    company.label,
    company.archived ? t("companies.withdrawn") : t("companies.active"),
    t("companies.areaCount", { count: company.areas }),
  ]
    .join(" ")
    .toLowerCase();
  return haystack.includes(query);
}
