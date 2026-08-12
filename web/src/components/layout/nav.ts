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
    label: "Operar",
    items: [
      { to: "/overview", label: "Visão geral", icon: LayoutDashboard, permission: "run:read" },
      { to: "/agents", label: "Agentes", icon: Bot, permission: "agent:read" },
      { to: "/runs", label: "Execuções", icon: Activity, permission: "run:read" },
      { to: "/approvals", label: "Fila humana", icon: Hand, permission: "approval:act" },
    ],
  },
  {
    label: "Governar",
    items: [
      { to: "/policies", label: "Políticas", icon: Scale, permission: "policy:read" },
      { to: "/audit", label: "Trilha de auditoria", icon: ScrollText, permission: "audit:read" },
      { to: "/cost", label: "Custo e limites", icon: Wallet, permission: "cost:read" },
      { to: "/integrations", label: "Integrações", icon: Plug, permission: "tool:read" },
      { to: "/admin", label: "Administração", icon: Settings2, permission: "tool:read" },
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

export const PAGE_TITLES: Record<string, string> = {
  overview: "Visão geral",
  agents: "Agentes",
  runs: "Execuções",
  approvals: "Fila humana",
  policies: "Políticas",
  audit: "Trilha de auditoria",
  cost: "Custo e limites",
  admin: "Administração",
  integrations: "Integrações",
};
