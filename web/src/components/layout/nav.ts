import {
  Activity,
  Bot,
  Hand,
  LayoutDashboard,
  Plug,
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
      { to: "/cost", label: "nav.cost", icon: Wallet, permission: "cost:read" },
      {
        to: "/integrations",
        label: "nav.integrations",
        icon: Plug,
        permission: "tool:read",
      },
      {
        to: "/admin",
        label: "nav.admin",
        icon: Settings2,
        permission: "tool:read",
      },
    ],
  },
];

/**
 * The icon each screen leads with.
 *
 * Beside the titles rather than inside every page, so a screen and its
 * navigation entry cannot drift into showing two different symbols for the
 * same thing.
 */
export const PAGE_ICONS: Record<string, LucideIcon> = {
  overview: LayoutDashboard,
  agents: Bot,
  runs: Activity,
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
  agents: "nav.agents",
  runs: "nav.runs",
  approvals: "nav.approvals",
  policies: "nav.policies",
  audit: "nav.audit",
  cost: "nav.cost",
  admin: "nav.admin",
  integrations: "nav.integrations",
};
