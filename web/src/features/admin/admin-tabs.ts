export const ADMIN_TAB_ACCESS = [
  {
    value: "tools",
    label: "admin.toolsWaiting",
    permission: "tool:classify",
  },
  {
    value: "events",
    label: "admin.trail",
    permission: "audit:read",
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
    value: "identity",
    label: "admin.identity",
    permission: "identity:write",
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
] as const;

export type AdminTab = (typeof ADMIN_TAB_ACCESS)[number];
export type AdminTabValue = AdminTab["value"];

export const ADMIN_TAB_GROUPS = [
  {
    label: "admin.group.activity",
    tabs: ["tools", "events"],
  },
  {
    label: "admin.group.platform",
    tabs: ["branding", "authoring", "identity"],
  },
  {
    label: "admin.group.organization",
    tabs: ["companies", "areas"],
  },
  {
    label: "admin.group.people",
    tabs: ["people"],
  },
  {
    label: "admin.group.limits",
    tabs: ["prices", "budgets", "retention"],
  },
] as const satisfies ReadonlyArray<{
  label: string;
  tabs: ReadonlyArray<AdminTabValue>;
}>;

const ADMIN_TABS_BY_VALUE = ADMIN_TAB_ACCESS.reduce(
  (byValue, tab) => ({ ...byValue, [tab.value]: tab }),
  {} as Record<AdminTabValue, AdminTab>,
);

export function visibleAdminTabs(
  can: string[] | null | undefined,
): AdminTab[] {
  if (can === null) return [...ADMIN_TAB_ACCESS];
  if (can === undefined) return [];
  return ADMIN_TAB_ACCESS.filter(
    (item) => can.includes(item.permission),
  );
}

export function visibleAdminTabGroups(
  can: string[] | null | undefined,
): Array<{ label: string; tabs: AdminTab[] }> {
  const visible = new Set(visibleAdminTabs(can).map((tab) => tab.value));

  return ADMIN_TAB_GROUPS.map((group) => ({
    label: group.label,
    tabs: group.tabs
      .map((value) => ADMIN_TABS_BY_VALUE[value])
      .filter((tab) => visible.has(tab.value)),
  })).filter((group) => group.tabs.length > 0);
}
