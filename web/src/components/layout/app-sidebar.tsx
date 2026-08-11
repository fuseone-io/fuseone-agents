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
import { NAV, type NavItem } from "@/components/layout/nav";
import { UserMenu } from "@/components/layout/user-menu";
import { useApprovals } from "@/features/approvals/api";
import { useMe } from "@/features/session/api";
import { LogoLockup, LogoMark } from "@/components/shared/logo";

export function AppSidebar() {
  const { pathname } = useLocation();
  const { data: me } = useMe();
  const allowed = permissionFilter(me?.can);
  const scope = scopeOf(me?.grants);

  // The one rule between the two grounds is the design system's sidebar
  // border, a step stronger than the hairline used everywhere else.
  return (
    <Sidebar collapsible="icon" className="border-r-sidebar-border">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" className="h-[46px] gap-[9px]" asChild>
              <Link to="/runs">
                {/* The mark sits in a box the width of the collapsed button, so
                    collapsing leaves the icon centred with nothing beside it.
                    A narrower box left a dozen pixels for the name to leak
                    through. The wrapper also keeps the mark out of reach of the
                    primitive's [&>svg]:size-4, which would shrink it to 16. */}
                <span className="flex size-8 shrink-0 items-center justify-center">
                  <LogoMark size={26} />
                </span>
                <div className="grid flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
                  <LogoLockup className="text-base text-sidebar-accent-foreground" />
                  {scope && <span className="truncate text-xs text-muted-foreground">{scope}</span>}
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        {NAV.map((group) => (
          <SidebarGroup key={group.label}>
            <SidebarGroupLabel className="text-2xs uppercase tracking-label">
              {group.label}
            </SidebarGroupLabel>
            <SidebarMenu>
              {group.items.filter(allowed).map((item) => (
                <NavLink key={item.to} item={item} active={pathname.startsWith(item.to)} />
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
  const Icon = item.icon;
  const pending = usePendingCount(item.to);

  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={item.label}>
        <Link to={item.to}>
          <Icon className={active ? "text-primary" : undefined} />
          <span>{item.label}</span>
        </Link>
      </SidebarMenuButton>
      {/* The count is what makes the queue worth opening; without it the
          approver has to check to find out there is nothing to do. */}
      {pending > 0 && <SidebarMenuBadge>{pending}</SidebarMenuBadge>}
    </SidebarMenuItem>
  );
}

function usePendingCount(to: string): number {
  const { data } = useApprovals();
  return to === "/approvals" ? (data?.items.length ?? 0) : 0;
}

/**
 * Before the session loads, nothing is hidden. A sidebar that reshuffles a
 * beat after the page paints is worse than one that briefly offers a link the
 * server will refuse.
 */
function permissionFilter(can: string[] | undefined) {
  return (item: NavItem) => !can || !item.permission || can.includes(item.permission);
}

// An installation with no identity configured has no scope to name, and an em
// dash there reads as a value that failed to load.
function scopeOf(grants: { company: string }[] | undefined): string {
  const companies = [...new Set(grants?.map((g) => g.company) ?? [])];
  if (companies.length > 1) return `${companies.length} empresas`;
  return companies[0] ?? "";
}
