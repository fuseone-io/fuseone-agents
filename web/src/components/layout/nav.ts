import { Activity, Bot, Hand, LayoutDashboard, Settings2, Wallet } from "lucide-react";
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
      { to: "/cost", label: "Custo e limites", icon: Wallet, permission: "cost:read" },
      { to: "/admin", label: "Administração", icon: Settings2, permission: "tool:read" },
    ],
  },
];

export const PAGE_TITLES: Record<string, string> = {
  overview: "Visão geral",
  agents: "Agentes",
  runs: "Execuções",
  approvals: "Fila humana",
  cost: "Custo e limites",
  admin: "Administração",
};
