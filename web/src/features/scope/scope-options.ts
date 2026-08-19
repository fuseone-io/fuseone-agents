import type { Company } from "@/features/companies/api";
import type { MeGrant } from "@/features/session/api";
import type { RegisteredScope } from "@/features/scope/api";

const INSTALLATION = "*";

export function scopeCompanies({
  grants,
  companies,
  areas,
}: {
  grants: MeGrant[];
  companies: Company[];
  areas: RegisteredScope[];
}) {
  const holdsInstallation = grants.some(
    (grant) => grant.company === INSTALLATION && grant.area === "",
  );
  const candidates = [
    ...(holdsInstallation ? companies.map((company) => company.id) : []),
    ...grants
      .map((grant) => grant.company)
      .filter((company) => company !== INSTALLATION),
    ...areas.map((area) => area.company),
  ];
  return [...new Set(candidates)].sort();
}
