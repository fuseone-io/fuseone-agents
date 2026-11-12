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
    // Read at the platform's scope only. An area's audit:read reads that
    // area's runs, on the audit screen, and never this trail.
    platform: true,
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
    // The assistant's provider is the platform's. An area's provider:write
    // configures its connector instances, not this (NT-019).
    platform: true,
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
    value: "serviceAccounts",
    label: "admin.serviceAccounts",
    permission: "identity:write",
  },
  {
    value: "standing",
    label: "admin.standing",
    permission: "approval:grant",
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
    tabs: ["people", "serviceAccounts"],
  },
  {
    label: "admin.group.limits",
    tabs: ["standing", "prices", "budgets", "retention"],
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
  // What the caller holds at the platform's scope. Absent from a server that
  // predates it, which then answers for can alone.
  platform?: string[],
): AdminTab[] {
  if (can === null) return [...ADMIN_TAB_ACCESS];
  if (can === undefined) return [];
  return ADMIN_TAB_ACCESS.filter(
    (item) =>
      can.includes(item.permission) &&
      (!("platform" in item) || !platform || platform.includes(item.permission)),
  );
}

export function visibleAdminTabGroups(
  can: string[] | null | undefined,
  platform?: string[],
): Array<{ label: string; tabs: AdminTab[] }> {
  const visible = new Set(
    visibleAdminTabs(can, platform).map((tab) => tab.value),
  );

  return ADMIN_TAB_GROUPS.map((group) => ({
    label: group.label,
    tabs: group.tabs
      .map((value) => ADMIN_TABS_BY_VALUE[value])
      .filter((tab) => visible.has(tab.value)),
  })).filter((group) => group.tabs.length > 0);
}
