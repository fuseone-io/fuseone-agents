import {
  BookOpen,
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
        permission: "tool:read",
      },
      { to: "/manual", label: "nav.manual", icon: BookOpen },
      {
        to: "/admin",
        label: "nav.admin",
        icon: Settings2,
        anyPermission: [
          "audit:read",
          "identity:write",
          "brand:write",
          "provider:write",
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
): boolean {
  if (can === null) return true;
  if (can === undefined) return !item.permission && !item.anyPermission;
  if (item.permission && !can.includes(item.permission)) return false;
  if (item.anyPermission) {
    return item.anyPermission.some((permission) => can.includes(permission));
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
