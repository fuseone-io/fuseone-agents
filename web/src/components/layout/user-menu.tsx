import { useTranslation } from "react-i18next";
import { ChevronsUpDown, LogOut } from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { csrfToken } from "@/lib/api/client";
import { useMe, type MeGrant } from "@/features/session/api";

export function UserMenu() {
  const { t } = useTranslation();
  const { data: me } = useMe();
  if (!me) return null;

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" className="h-11">
              {/* Same width as the collapsed button, for the same reason as the
                  header: the avatar is what remains, and nothing leaks past it. */}
              <span className="flex size-8 shrink-0 items-center justify-center">
                <Avatar className="size-6 rounded-sm">
                  <AvatarFallback className="rounded-sm bg-sidebar-accent text-2xs font-semibold">
                    {initialsOf(me.display)}
                  </AvatarFallback>
                </Avatar>
              </span>
              <div className="grid flex-1 text-left leading-tight group-data-[collapsible=icon]:hidden">
                <span className="truncate text-sm font-medium">
                  {me.display}
                </span>
                {/* The design shows an email here. This console shows the role
                    instead: in a governance tool, what somebody may do is the
                    fact a reader needs at a glance. */}
                <span className="truncate text-xs text-muted-foreground">
                  {rolesOf(me.grants)}
                </span>
              </div>
              <ChevronsUpDown className="size-3.5 text-muted-foreground group-data-[collapsible=icon]:hidden" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>

          <DropdownMenuContent side="right" align="end" className="w-56">
            <DropdownMenuLabel className="font-normal text-muted-foreground">
              {me.display}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={signOut}>
              <LogOut className="size-4" />
              {t("shell.signOut")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

/**
 * Signing out revokes the session on the server, so a copied cookie stops
 * working too. A full reload afterwards clears every cached answer the old
 * session produced rather than leaving them in memory.
 */
async function signOut() {
  const token = csrfToken();
  await fetch("/auth/logout", {
    method: "POST",
    credentials: "same-origin",
    headers: token ? { "X-CSRF-Token": token } : undefined,
  });
  globalThis.location.assign("/");
}

function initialsOf(display: string): string {
  const words = display.trim().split(/\s+/).slice(0, 2);
  return words.map((w) => w[0]?.toUpperCase() ?? "").join("") || "?";
}

function rolesOf(grants: MeGrant[]): string {
  const roles = [...new Set(grants.map((g) => g.role))];
  return roles.length ? roles.join(", ") : "Sem acesso";
}
