export const ADMIN_TAB_ACCESS = [
  {
    value: "tools",
    label: "admin.toolsWaiting",
    permission: "tool:classify",
  },
  {
    value: "branding",
    label: "admin.branding",
    permission: "brand:write",
  },
  {
    value: "authoring",
    label: "admin.authoring",
    permission: "provider:write",
  },
  {
    value: "companies",
    label: "companies.companies",
    permission: "company:write",
  },
  {
    value: "areas",
    label: "admin.areas",
    permission: "scope:write",
  },
  {
    value: "identity",
    label: "admin.identity",
    permission: "identity:write",
  },
  {
    value: "people",
    label: "admin.people",
    permission: "identity:write",
  },
  {
    value: "prices",
    label: "admin.prices",
    permission: "budget:write",
  },
  {
    value: "budgets",
    label: "admin.budgets",
    permission: "budget:write",
  },
  {
    value: "retention",
    label: "admin.retention",
    permission: "data:erase",
  },
  {
    value: "events",
    label: "admin.trail",
    permission: "audit:read",
  },
] as const;

export type AdminTab = (typeof ADMIN_TAB_ACCESS)[number];
export type AdminTabValue = AdminTab["value"];

export function visibleAdminTabs(can: string[] | undefined): AdminTab[] {
  return ADMIN_TAB_ACCESS.filter(
    (item) => !can || can.includes(item.permission),
  );
}
