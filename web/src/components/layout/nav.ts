import {
  BookOpen,
  Bookmark,
  Activity,
  Bot,
  Brain,
  Hand,
  LayoutDashboard,
  Plug,
  ServerCog,
  Scale,
  ScrollText,
  Settings2,
  Wallet,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

export interface NavItem {
  to: string;
  /** A catalogue key. The label is resolved where it is rendered — a label
   *  read at module load would be pinned to whichever language loaded first. */
  label: string;
  icon: LucideIcon;
  /**
   * Permission the item needs to be worth showing. Hiding is a courtesy, not
   * a control: the server checks again on every request, where the scope of
   * the specific resource is known.
   */
  permission?: string;
  /**
   * At least one of these permissions must be present. Used for hub pages that
   * contain several administrative surfaces; showing them for a common read
   * permission teaches people that the console offers acts they cannot take.
   */
  anyPermission?: string[];
  /**
   * A permission the item needs at the platform's administration scope, not
   * just somewhere. For a screen the server only serves there, such as the
   * integrations: an administrator of one area holds tool:read in that area,
   * and every request the screen makes would answer 403.
   */
  platformPermission?: string;
  /**
   * Like anyPermission, held at the platform's administration scope. For a
   * permission whose only surface in a hub is served there: an area's
   * audit:read reads that area's runs, and opens no administration tab.
   */
  platformAnyPermission?: string[];
  /**
   * Shows the item whatever the rest says, when held anywhere. For a screen
   * with one part an area may use: an area's administrator configures its own
   * connector instances, under integrations that are otherwise the platform's
   * (NT-019).
   */
  orPermission?: string;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

/**
 * Grouped the way the design system groups them — what you operate, what you
 * govern — so the sidebar reads as two jobs rather than one long list.
 *
 * Only routes that exist appear here. A navigation entry pointing at a screen
 * that has not been built teaches people the console is unreliable.
 */
export const NAV: NavGroup[] = [
  {
    label: "nav.operate",
    items: [
      {
        to: "/overview",
        label: "nav.overview",
        icon: LayoutDashboard,
        permission: "run:read",
      },
      {
        to: "/agents",
        label: "nav.agents",
        icon: Bot,
        permission: "agent:read",
      },
      {
        to: "/runs",
        label: "nav.runs",
        icon: Activity,
        permission: "run:read",
      },
      {
        to: "/runtime",
        label: "nav.runtime",
        icon: ServerCog,
        permission: "run:read",
      },
      {
        to: "/approvals",
        label: "nav.approvals",
        icon: Hand,
        permission: "approval:act",
      },
    ],
  },
  {
    label: "nav.govern",
    items: [
      {
        to: "/policies",
        label: "nav.policies",
        icon: Scale,
        permission: "policy:read",
      },
      {
        to: "/audit",
        label: "nav.audit",
        icon: ScrollText,
        permission: "audit:read",
      },
      {
        to: "/memory",
        label: "nav.memory",
        icon: Brain,
        permission: "agent:read",
      },
      { to: "/cost", label: "nav.cost", icon: Wallet, permission: "cost:read" },
      {
        to: "/integrations",
        label: "nav.integrations",
        icon: Plug,
        // Whoever reads tools somewhere keeps their own credentials there,
        // and an area's configurer its servers and connectors (NT-019). The
        // gate opens each the sections that are theirs.
        permission: "tool:read",
        orPermission: "provider:write",
      },
      { to: "/manual", label: "nav.manual", icon: BookOpen },
      {
        to: "/admin",
        label: "nav.admin",
        icon: Settings2,
        // audit:read opens only the administrative trail, which is the
        // platform's, so it counts when held there.
        // provider:write too: in an area it configures connector
        // instances, and opens no administration tab (NT-019).
        platformAnyPermission: ["audit:read", "provider:write"],
        anyPermission: [
          "identity:write",
          "brand:write",
          "budget:write",
          "policy:write",
          "scope:write",
          "data:erase",
          "company:write",
          "tool:classify",
        ],
      },
    ],
  },
];

export function navItemVisible(
  item: NavItem,
  can: string[] | null | undefined,
  // Absent from a server that predates it, which then answers for can alone.
  platform?: string[],
): boolean {
  if (can === null) return true;
  if (can === undefined) {
    return !item.permission && !item.anyPermission && !item.platformAnyPermission;
  }
  if (item.orPermission && can.includes(item.orPermission)) return true;
  if (item.permission && !can.includes(item.permission)) return false;
  if (
    item.platformPermission &&
    platform &&
    !platform.includes(item.platformPermission)
  ) {
    return false;
  }
  if (item.anyPermission || item.platformAnyPermission) {
    // Absent from a server that predates it, platform falls back to can.
    const atPlatform = platform ?? can;
    return (
      (item.anyPermission ?? []).some((p) => can.includes(p)) ||
      (item.platformAnyPermission ?? []).some((p) => atPlatform.includes(p))
    );
  }
  return true;
}

/**
 * The icon each screen leads with.
 *
 * Beside the titles rather than inside every page, so a screen and its
 * navigation entry cannot drift into showing two different symbols for the
 * same thing.
 */
export const PAGE_ICONS: Record<string, LucideIcon> = {
  manual: BookOpen,
  memory: Brain,
  overview: LayoutDashboard,
  agents: Bot,
  runs: Activity,
  runtime: ServerCog,
  approvals: Hand,
  policies: Scale,
  audit: ScrollText,
  cost: Wallet,
  admin: Settings2,
  integrations: Plug,
  // Reached from a run's claim, never from the navigation: there is no list
  // of cards to start from yet. The symbol is the claim's own in the trail.
  "work-items": Bookmark,
};

/** The catalogue key for each screen's name, so the breadcrumb and the
 *  navigation cannot drift into calling one screen two things. */
export const PAGE_TITLES: Record<string, string> = {
  overview: "nav.overview",
  manual: "nav.manual",
  memory: "nav.memory",
  agents: "nav.agents",
  runs: "nav.runs",
  runtime: "nav.runtime",
  approvals: "nav.approvals",
  policies: "nav.policies",
  audit: "nav.audit",
  cost: "nav.cost",
  admin: "nav.admin",
  integrations: "nav.integrations",
  "work-items": "workItems.title",
};

/**
 * Sub-screens that have a name rather than an identifier.
 *
 * Everything else in a second breadcrumb position is a record — a run, an
 * agent, a policy code — and reads in mono because it is machine-generated.
 */
const SUB_TITLES: Record<string, Record<string, string>> = {
  agents: {
    new: "agents.newAgent",
    interview: "interview.title",
    edit: "agents.edit",
  },
  policies: {
    new: "policies.newPolicy",
  },
};

export function subTitleKey(section: string, detail: string) {
  return SUB_TITLES[section]?.[detail];
}
