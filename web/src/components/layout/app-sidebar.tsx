import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { NAV, navItemVisible, type NavItem } from "@/components/layout/nav";
import { UserMenu } from "@/components/layout/user-menu";
import { useApprovals } from "@/features/approvals/api";
import { useMe } from "@/features/session/api";
import { ScopeSwitcher } from "@/features/scope/scope-switcher";

export function AppSidebar() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { data: me } = useMe();
  const can = me === null ? null : me?.can;
  const allowed = (item: NavItem) => navItemVisible(item, can);

  // The one rule between the two grounds is the design system's sidebar
  // border, a step stronger than the hairline used everywhere else.
  return (
    <Sidebar collapsible="icon" className="border-r-sidebar-border">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            {/* The header used to be a link home carrying the company as a
                label. A label is the wrong shape for it: the company is one of
                several contexts a person may hold, and the one thing they
                could not do was change it. */}
            <ScopeSwitcher />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        {NAV.map((group) => (
          <SidebarGroup key={group.label}>
            <SidebarGroupLabel className="text-2xs uppercase tracking-label">
              {t(group.label)}
            </SidebarGroupLabel>
            <SidebarMenu>
              {group.items.filter(allowed).map((item) => (
                <NavLink
                  key={item.to}
                  item={item}
                  active={pathname.startsWith(item.to)}
                />
              ))}
            </SidebarMenu>
          </SidebarGroup>
        ))}
      </SidebarContent>

      <SidebarFooter>
        <UserMenu />
      </SidebarFooter>
    </Sidebar>
  );
}

function NavLink({ item, active }: { item: NavItem; active: boolean }) {
  const { t } = useTranslation();
  const Icon = item.icon;
  const label = t(item.label);
  const pending = usePendingCount(item.to);

  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={label}>
        <Link to={item.to}>
          <Icon className={active ? "text-primary" : undefined} />
          <span>{label}</span>
        </Link>
      </SidebarMenuButton>
      {/* The count is what makes the queue worth opening; without it the
          approver has to check to find out there is nothing to do. */}
      {pending > 0 && <SidebarMenuBadge>{pending}</SidebarMenuBadge>}
    </SidebarMenuItem>
  );
}

function usePendingCount(to: string): number {
  const { items } = useApprovals();
  return to === "/approvals" ? items.length : 0;
}

// An installation with no identity configured has no scope to name, and an em
// dash there reads as a value that failed to load.
