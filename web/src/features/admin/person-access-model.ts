import type { TFunction } from "i18next";
import type { HeldGrant } from "@/features/admin/people-api";

export type Role = HeldGrant["role"];
export type Origin = "local" | "provider" | "mixed";

export type ScopeGrant = {
  scope: string;
  roles: Role[];
  origin: Origin;
};

export const ROLE_ORDER: Role[] = [
  "admin",
  "approver",
  "auditor",
  "author",
  "curator",
];

export function groupGrants(grants: HeldGrant[]): ScopeGrant[] {
  const byScope = new Map<
    string,
    { roles: Set<Role>; origins: Set<"local" | "provider"> }
  >();

  for (const grant of grants) {
    const scope = scopeOf(grant);
    const existing =
      byScope.get(scope) ??
      ({ roles: new Set<Role>(), origins: new Set<"local" | "provider">() });
    existing.roles.add(grant.role);
    existing.origins.add(grant.asserted ? "provider" : "local");
    byScope.set(scope, existing);
  }

  return [...byScope.entries()]
    .map(([scope, value]) => ({
      scope,
      roles: ROLE_ORDER.filter((role) => value.roles.has(role)),
      origin:
        value.origins.size > 1
          ? ("mixed" as const)
          : value.origins.has("provider")
            ? ("provider" as const)
            : ("local" as const),
    }))
    .sort((a, b) => a.scope.localeCompare(b.scope));
}

export function roleSummary(roles: Role[], t: TFunction) {
  if (roles.includes("admin")) return t("roles.admin").toLocaleLowerCase();
  if (roles.length === ROLE_ORDER.length - 1) return t("people.fullAccess");
  if (roles.length === 1) return t(`roles.${roles[0]}`).toLocaleLowerCase();
  return t("people.roleCount", { count: roles.length });
}

function scopeOf(grant: HeldGrant) {
  return grant.area === "" ? grant.company : `${grant.company}/${grant.area}`;
}
